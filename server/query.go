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
	"time"
	"unicode"

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
	tokens, err := tokenize(req.r.PostForm.Get("query"))
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, errors.New("Empty query")
	}

	typ := "hosts"
	var args string
	for i, tok := range tokens {
		if len(args) > 0 {
			args += " AND"
		}

		if fields := strings.SplitN(tok, ":", 2); len(fields) == 2 {
			// Query: [<type>:] [<sibling-type>.]<attribute>:<value> ...
			if i == 0 && fields[1] == "" {
				typ = fields[0]
			} else if elems := strings.Split(fields[0], "."); len(elems) > 1 {
				objs := elems[:len(elems)-1]
				for _, o := range objs {
					if o != "host" && o != "service" && o != "metric" {
						return nil, fmt.Errorf("Invalid object type %q", o)
					}
				}
				args += fmt.Sprintf(" %s.attribute[%s] = %s",
					strings.Join(objs, "."), proto.EscapeString(elems[len(elems)-1]),
					proto.EscapeString(fields[1]))
			} else {
				args += fmt.Sprintf(" attribute[%s] = %s",
					proto.EscapeString(fields[0]), proto.EscapeString(fields[1]))
			}
		} else {
			args += fmt.Sprintf(" name =~ %s", proto.EscapeString(tok))
		}
	}

	res, err := s.query("LOOKUP %s MATCHING"+args, identifier(typ))
	if err != nil {
		return nil, err
	}
	if t, ok := s.results[typ]; ok {
		return tmpl(t, res)
	}
	return nil, fmt.Errorf("Unsupported type %s", typ)
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
		if err == nil && req.cmd == "metric" {
			return metric(req, res, s)
		}
	default:
		panic("Unknown request: fetch(" + req.cmd + ")")
	}
	if err != nil {
		return nil, err
	}
	return tmpl(s.results[req.cmd], res)
}

var datetime = "2006-01-02 15:04:05"

func metric(req request, res interface{}, s *Server) (*page, error) {
	start := time.Now().Add(-24 * time.Hour)
	end := time.Now()
	if req.r.Method == "POST" {
		var err error
		// Parse the values first to verify their format.
		if s := req.r.PostForm.Get("start_date"); s != "" {
			if start, err = time.Parse(datetime, s); err != nil {
				return nil, fmt.Errorf("Invalid start time %q", s)
			}
		}
		if e := req.r.PostForm.Get("end_date"); e != "" {
			if end, err = time.Parse(datetime, e); err != nil {
				return nil, fmt.Errorf("Invalid end time %q", e)
			}
		}
	}

	p := struct {
		StartTime string
		EndTime   string
		URLStart  string
		URLEnd    string
		Data      interface{}
	}{
		start.Format(datetime),
		end.Format(datetime),
		start.Format(urldate),
		end.Format(urldate),
		res,
	}
	return tmpl(s.results["metric"], &p)
}

// tokenize split the string s into its tokens where a token is either a quoted
// string or surrounded by one or more consecutive whitespace characters.
func tokenize(s string) ([]string, error) {
	scan := scanner{}
	tokens := []string{}
	start := -1
	for i, r := range s {
		if !scan.inField(r) {
			if start == -1 {
				// Skip leading and consecutive whitespace.
				continue
			}
			tok, err := unescape(s[start:i])
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			start = -1
		} else if start == -1 {
			// Found a new field.
			start = i
		}
	}
	if start >= 0 {
		// Last (or possibly only) field.
		tok, err := unescape(s[start:])
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
	}

	if scan.inQuotes {
		return nil, errors.New("quoted string not terminated")
	}
	if scan.escaped {
		return nil, errors.New("illegal character escape at end of string")
	}
	return tokens, nil
}

func unescape(s string) (string, error) {
	var unescaped []byte
	var i, n int
	for i = 0; i < len(s); i++ {
		if s[i] != '\\' {
			n++
			continue
		}

		if i >= len(s) {
			return "", errors.New("illegal character escape at end of string")
		}
		if s[i+1] != ' ' && s[i+1] != '"' && s[i+1] != '\\' {
			// Allow simple escapes only for now.
			return "", fmt.Errorf("illegal character escape \\%c", s[i+1])
		}
		if unescaped == nil {
			unescaped = []byte(s)
		}
		copy(unescaped[n:], s[i+1:])
	}

	if unescaped != nil {
		return string(unescaped[:n]), nil
	}
	return s, nil
}

type scanner struct {
	inQuotes bool
	escaped  bool
}

func (s *scanner) inField(r rune) bool {
	if s.escaped {
		s.escaped = false
		return true
	}
	if r == '\\' {
		s.escaped = true
		return true
	}
	if s.inQuotes {
		if r == '"' {
			s.inQuotes = false
			return false
		}
		return true
	}
	if r == '"' {
		s.inQuotes = true
		return false
	}
	return !unicode.IsSpace(r)
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
		case time.Time:
			args[i] = v.Format(datetime)
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
