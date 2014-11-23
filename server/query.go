//
// Copyright (C) 2014 Sebastian 'tokkee' Harl <sh@tokkee.org>
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions
// are met:
// 1. Redistributions of source code must retain the above copyright
//    notice, this list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright
//    notice, this list of conditions and the following disclaimer in the
//    documentation and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// ``AS IS'' AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED
// TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR
// PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDERS OR
// CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL,
// EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
// PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS;
// OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY,
// WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR
// OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF
// ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package server

// Helper functions for handling queries.

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/sysdb/go/proto"
	"github.com/sysdb/go/sysdb"
)

func listAll(req request, s *Server) (*page, error) {
	if len(req.args) != 0 {
		return nil, fmt.Errorf("%s not found", strings.Title(req.cmd))
	}

	res, err := s.query("LIST %s", identifier(req.cmd))
	if err != nil {
		return nil, err
	}
	// the template *must* exist
	return tmpl(s.results[req.cmd], res)
}

func lookup(req request, s *Server) (*page, error) {
	if req.r.Method != "POST" {
		return nil, errors.New("Method not allowed")
	}
	q := req.r.FormValue("query")
	if q == "''" {
		return nil, errors.New("Empty query")
	}

	res, err := s.query("LOOKUP hosts MATCHING name =~ %s", q)
	if err != nil {
		return nil, err
	}
	return tmpl(s.results["hosts"], res)
}

func fetch(req request, s *Server) (*page, error) {
	if len(req.args) == 0 {
		return nil, fmt.Errorf("%s not found", strings.Title(req.cmd))
	}

	var res interface{}
	var err error
	switch req.cmd {
	case "host":
		if len(req.args) != 1 {
			return nil, fmt.Errorf("%s not found", strings.Title(req.cmd))
		}
		res, err = s.query("FETCH host %s", req.args[0])
	case "service", "metric":
		if len(req.args) != 2 {
			return nil, fmt.Errorf("%s not found", strings.Title(req.cmd))
		}
		res, err = s.query("FETCH %s %s.%s", identifier(req.cmd), req.args[0], req.args[1])
	default:
		panic("Unknown request: fetch(" + req.cmd + ")")
	}
	if err != nil {
		return nil, err
	}
	return tmpl(s.results[req.cmd], res)
}

type identifier string

func (s *Server) query(cmd string, args ...interface{}) (interface{}, error) {
	c := <-s.conns
	defer func() { s.conns <- c }()

	for i, arg := range args {
		switch v := arg.(type) {
		case identifier:
			// Nothing to do.
		case string:
			args[i] = proto.EscapeString(v)
		default:
			panic(fmt.Sprintf("query: invalid type %T", arg))
		}
	}

	cmd = fmt.Sprintf(cmd, args...)
	m := &proto.Message{
		Type: proto.ConnectionQuery,
		Raw:  []byte(cmd),
	}
	if err := c.Send(m); err != nil {
		return nil, fmt.Errorf("Query %q: %v", cmd, err)
	}

	for {
		m, err := c.Receive()
		if err != nil {
			return nil, fmt.Errorf("Failed to receive server response: %v", err)
		}
		if m.Type == proto.ConnectionLog {
			log.Println(string(m.Raw[4:]))
			continue
		} else if m.Type == proto.ConnectionError {
			return nil, errors.New(string(m.Raw))
		}

		t, err := m.DataType()
		if err != nil {
			return nil, fmt.Errorf("Failed to unmarshal response: %v", err)
		}

		var res interface{}
		switch t {
		case proto.HostList:
			var hosts []sysdb.Host
			err = proto.Unmarshal(m, &hosts)
			res = hosts
		case proto.Host:
			var host sysdb.Host
			err = proto.Unmarshal(m, &host)
			res = host
		case proto.Timeseries:
			var ts sysdb.Timeseries
			err = proto.Unmarshal(m, &ts)
			res = ts
		default:
			return nil, fmt.Errorf("Unsupported data type %d", t)
		}
		if err != nil {
			return nil, fmt.Errorf("Failed to unmarshal response: %v", err)
		}
		return res, nil
	}
}

// vim: set tw=78 sw=4 sw=4 noexpandtab :
