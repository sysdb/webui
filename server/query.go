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
	"strings"
	"time"
	"unicode"

	"github.com/sysdb/go/client"
	"github.com/sysdb/go/proto"
)

func listAll(req request, s *Server) (*page, error) {
	if len(req.args) != 0 {
		return nil, fmt.Errorf("%s not found", strings.Title(req.cmd))
	}

	q, err := client.QueryString("LIST %s", client.Identifier(req.cmd))
	if err != nil {
		return nil, err
	}
	res, err := s.c.Query(q)
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
	raw, err := parseQuery(req.r.PostForm.Get("query"))
	if err != nil {
		return nil, err
	}

	if raw.typ == "" {
		raw.typ = "hosts"
	}
	var args string
	for name, value := range raw.args {
		if len(args) > 0 {
			args += " AND"
		}

		if name == "name" {
			args += fmt.Sprintf(" name =~ %s", value)
		} else {
			args += fmt.Sprintf(" %s = %s", name, value)
		}
	}

	q, err := client.QueryString("LOOKUP %s MATCHING"+args, client.Identifier(raw.typ))
	if err != nil {
		return nil, err
	}
	res, err := s.c.Query(q)
	if err != nil {
		return nil, err
	}
	if t, ok := s.results[raw.typ]; ok {
		return tmpl(t, res)
	}
	return nil, fmt.Errorf("Unsupported type %s", raw.typ)
}

func fetch(req request, s *Server) (*page, error) {
	if len(req.args) == 0 {
		return nil, fmt.Errorf("%s not found", strings.Title(req.cmd))
	}

	var q string
	var err error
	switch req.cmd {
	case "host":
		if len(req.args) != 1 {
			return nil, fmt.Errorf("%s not found", strings.Title(req.cmd))
		}
		q, err = client.QueryString("FETCH host %s", req.args[0])
	case "service", "metric":
		if len(req.args) != 2 {
			return nil, fmt.Errorf("%s not found", strings.Title(req.cmd))
		}
		q, err = client.QueryString("FETCH %s %s.%s", client.Identifier(req.cmd), req.args[0], req.args[1])
	default:
		panic("Unknown request: fetch(" + req.cmd + ")")
	}
	if err != nil {
		return nil, err
	}

	res, err := s.c.Query(q)
	if err != nil {
		return nil, err
	}
	if req.cmd == "metric" {
		return metric(req, res, s)
	}
	return tmpl(s.results[req.cmd], res)
}

func graphs(req request, s *Server) (*page, error) {
	p := struct {
		Query, Metrics string
		QueryOptions   string
		GroupBy        string
	}{
		Query:   req.r.PostForm.Get("metrics-query"),
		GroupBy: req.r.PostForm.Get("group-by"),
	}

	if req.r.Method == "POST" {
		p.Metrics = p.Query
		if p.GroupBy != "" {
			p.QueryOptions += "/g=" + strings.Join(strings.Fields(p.GroupBy), ",")
		}
	}
	return tmpl(s.results["graphs"], &p)
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

type query struct {
	typ  string
	args map[string]string
}

func (q *query) arg(name, value string) error {
	if _, ok := q.args[name]; ok {
		return fmt.Errorf("Duplicate key %q", name)
	}
	q.args[name] = proto.EscapeString(value)
	return nil
}

func (q *query) attr(parent, name, value string) error {
	var k string
	if parent != "" {
		k = fmt.Sprintf("%s.attribute[%s]", parent, proto.EscapeString(name))
	} else {
		k = fmt.Sprintf("attribute[%s]", proto.EscapeString(name))
	}

	return q.arg(k, value)
}

func parseQuery(s string) (*query, error) {
	tokens, err := tokenize(s)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, errors.New("Empty query")
	}

	q := &query{args: make(map[string]string)}
	for i, tok := range tokens {
		if fields := strings.SplitN(tok, ":", 2); len(fields) == 2 {
			// Query: [<type>:] [<sibling-type>.]<attribute>:<value> ...
			if i == 0 && fields[1] == "" {
				q.typ = fields[0]
			} else if elems := strings.Split(fields[0], "."); len(elems) > 1 {
				objs := elems[:len(elems)-1]
				for _, o := range objs {
					if o != "host" && o != "service" && o != "metric" {
						return nil, fmt.Errorf("Invalid object type %q", o)
					}
				}
				if err := q.attr(strings.Join(objs, "."), elems[len(elems)-1], fields[1]); err != nil {
					return nil, err
				}
			} else {
				if err := q.attr("", fields[0], fields[1]); err != nil {
					return nil, err
				}
			}
		} else {
			if err := q.arg("name", tok); err != nil {
				return nil, err
			}
		}
	}
	return q, nil
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

// vim: set tw=78 sw=4 sw=4 noexpandtab :
