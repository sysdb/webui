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

// Package server implements the core of the SysDB web server.
package server

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/sysdb/go/client"
	"github.com/sysdb/go/proto"
	"github.com/sysdb/go/sysdb"
)

// A Config specifies configuration values for a SysDB web server.
type Config struct {
	// Conn specifies a connection to a SysDB server instance.
	Conn *client.Conn

	// TemplatePath specifies the relative or absolute location of template files.
	TemplatePath string

	// StaticPath specifies the relative or absolute location of static files.
	StaticPath string
}

// A Server implements an http.Handler that serves the SysDB user interface.
type Server struct {
	c *client.Conn

	// Templates:
	main    *template.Template
	results map[string]*template.Template

	// Static content:
	static http.Handler
}

// New constructs a new SysDB web server using the specified configuration.
func New(cfg Config) (*Server, error) {
	s := &Server{c: cfg.Conn, results: make(map[string]*template.Template)}

	var err error
	s.main, err = cfg.parse("main.tmpl")
	if err != nil {
		return nil, err
	}

	types := []string{"host", "hosts", "service", "services", "metric", "metrics"}
	for _, t := range types {
		s.results[t], err = cfg.parse(t + ".tmpl")
		if err != nil {
			return nil, err
		}
	}

	s.static = http.FileServer(http.Dir(cfg.StaticPath))
	return s, nil
}

func (cfg Config) parse(name string) (*template.Template, error) {
	t := template.New(filepath.Base(name))
	return t.ParseFiles(filepath.Join(cfg.TemplatePath, name))
}

type request struct {
	r    *http.Request
	cmd  string
	args []string
}

var handlers = map[string]func(request, *Server) (template.HTML, error){
	"": index,

	// Queries
	"host":     fetch,
	"service":  fetch,
	"metric":   fetch,
	"hosts":    listAll,
	"services": listAll,
	"metrics":  listAll,
	"lookup":   lookup,
}

// ServeHTTP implements the http.Handler interface and serves
// the SysDB user interface.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	fields := strings.Split(path, "/")

	if fields[0] == "style" || fields[0] == "images" {
		s.static.ServeHTTP(w, r)
		return
	}

	req := request{
		r:   r,
		cmd: fields[0],
	}
	if len(fields) > 1 {
		if fields[len(fields)-1] == "" {
			// Slash at the end of the URL
			fields = fields[:len(fields)-1]
		}
		if len(fields) > 1 {
			req.args = fields[1:]
		}
	}

	f, ok := handlers[req.cmd]
	if !ok {
		s.notfound(w, r)
		return
	}
	r.ParseForm()
	content, err := f(req, s)
	if err != nil {
		s.err(w, http.StatusBadRequest, fmt.Errorf("Error: %v", err))
		return
	}

	page := struct {
		Title   string
		Query   string
		Content template.HTML
	}{
		Title:   "SysDB - The System Database",
		Query:   r.FormValue("query"),
		Content: content,
	}

	var buf bytes.Buffer
	err = s.main.Execute(&buf, &page)
	if err != nil {
		s.internal(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	io.Copy(w, &buf)
}

// Content handlers.

func index(_ request, s *Server) (template.HTML, error) {
	return "<section><h1>Welcome to the System Database.</h1></section>", nil
}

func listAll(req request, s *Server) (template.HTML, error) {
	if len(req.args) != 0 {
		return "", fmt.Errorf("%s not found", strings.Title(req.cmd))
	}

	res, err := s.query(fmt.Sprintf("LIST %s", req.cmd))
	if err != nil {
		return "", err
	}
	// the template *must* exist
	return tmpl(s.results[req.cmd], res)
}

func lookup(req request, s *Server) (template.HTML, error) {
	if req.r.Method != "POST" {
		return "", errors.New("Method not allowed")
	}
	q := proto.EscapeString(req.r.FormValue("query"))
	if q == "''" {
		return "", errors.New("Empty query")
	}

	res, err := s.query(fmt.Sprintf("LOOKUP hosts MATCHING name =~ %s", q))
	if err != nil {
		return "", err
	}
	return tmpl(s.results["hosts"], res)
}

func fetch(req request, s *Server) (template.HTML, error) {
	if len(req.args) == 0 {
		return "", fmt.Errorf("%s not found", strings.Title(req.cmd))
	}

	var q string
	switch req.cmd {
	case "host":
		if len(req.args) != 1 {
			return "", fmt.Errorf("%s not found", strings.Title(req.cmd))
		}
		q = fmt.Sprintf("FETCH host %s", proto.EscapeString(req.args[0]))
	case "service", "metric":
		if len(req.args) < 2 {
			return "", fmt.Errorf("%s not found", strings.Title(req.cmd))
		}
		host := proto.EscapeString(req.args[0])
		name := proto.EscapeString(strings.Join(req.args[1:], "/"))
		q = fmt.Sprintf("FETCH %s %s.%s", req.cmd, host, name)
	default:
		panic("Unknown request: fetch(" + req.cmd + ")")
	}

	res, err := s.query(q)
	if err != nil {
		return "", err
	}
	return tmpl(s.results[req.cmd], res)
}

func tmpl(t *template.Template, data interface{}) (template.HTML, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("Template error: %v", err)
	}
	return template.HTML(buf.String()), nil
}

func html(s string) template.HTML {
	return template.HTML(template.HTMLEscapeString(s))
}

func (s *Server) query(cmd string) (interface{}, error) {
	m := &proto.Message{
		Type: proto.ConnectionQuery,
		Raw:  []byte(cmd),
	}
	if err := s.c.Send(m); err != nil {
		return nil, fmt.Errorf("Query %q: %v", cmd, err)
	}

	for {
		m, err := s.c.Receive()
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
