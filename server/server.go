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
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/sysdb/go/client"
)

// A Config specifies configuration values for a SysDB web server.
type Config struct {
	// TemplatePath specifies the relative or absolute location of template files.
	TemplatePath string

	// StaticPath specifies the relative or absolute location of static files.
	StaticPath string
}

// A Server implements an http.Handler that serves the SysDB user interface.
type Server struct {
	c *client.Client

	// Request multiplexer
	mux map[string]handler

	// Templates:
	main    *template.Template
	results map[string]*template.Template

	// Base directory of static files.
	basedir string
}

// New constructs a new SysDB web server using the specified configuration.
func New(addr, user string, cfg Config) (*Server, error) {
	s := &Server{results: make(map[string]*template.Template)}

	var err error
	if s.c, err = client.Connect(addr, user); err != nil {
		return nil, err
	}
	if major, minor, patch, extra, err := s.c.ServerVersion(); err == nil {
		log.Printf("Connected to SysDB %d.%d.%d%s.", major, minor, patch, extra)
	}

	if s.main, err = cfg.parse("main.tmpl"); err != nil {
		return nil, err
	}
	types := []string{"graphs", "host", "hosts", "service", "services", "metric", "metrics"}
	for _, t := range types {
		s.results[t], err = cfg.parse(t + ".tmpl")
		if err != nil {
			return nil, err
		}
	}

	s.basedir = cfg.StaticPath
	s.mux = map[string]handler{
		"images": s.static,
		"style":  s.static,
		"graph":  s.graph,
	}
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

type handler func(http.ResponseWriter, request)

type page struct {
	Title   string
	Query   string
	Content template.HTML
}

// Content generators for HTML pages.
var content = map[string]func(request, *Server) (*page, error){
	"": index,

	// Queries
	"graphs":   graphs,
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
	path := r.RequestURI
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	var fields []string
	for _, f := range strings.Split(path, "/") {
		f, err := url.QueryUnescape(f)
		if err != nil {
			s.badrequest(w, fmt.Errorf("Error: %v", err))
			return
		}
		fields = append(fields, f)
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

	if h := s.mux[fields[0]]; h != nil {
		h(w, req)
		return
	}

	f, ok := content[req.cmd]
	if !ok {
		s.notfound(w, r)
		return
	}
	r.ParseForm()
	p, err := f(req, s)
	if err != nil {
		p = &page{
			Content: "<section class=\"error\">" +
				html(fmt.Sprintf("Error: %v", err)) +
				"</section>",
		}
	}

	p.Query = r.FormValue("query")
	if p.Title == "" {
		p.Title = "SysDB - The System Database"
	}

	var buf bytes.Buffer
	err = s.main.Execute(&buf, p)
	if err != nil {
		s.internal(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	io.Copy(w, &buf)
}

// static serves static content.
func (s *Server) static(w http.ResponseWriter, req request) {
	http.ServeFile(w, req.r, filepath.Clean(filepath.Join(s.basedir, req.r.URL.Path)))
}

func index(_ request, s *Server) (*page, error) {
	major, minor, patch, extra, err := s.c.ServerVersion()
	if err != nil {
		return nil, err
	}

	content := fmt.Sprintf("<section>"+
		"<h1>Welcome to the System Database.</h1>"+
		"<p>Connected to SysDB %d.%d.%d%s</p>"+
		"</section>", major, minor, patch, html(extra))
	return &page{Content: template.HTML(content)}, nil
}

func tmpl(t *template.Template, data interface{}) (*page, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("Template error: %v", err)
	}
	return &page{Content: template.HTML(buf.String())}, nil
}

func html(s string) template.HTML {
	return template.HTML(template.HTMLEscapeString(s))
}

// vim: set tw=78 sw=4 sw=4 noexpandtab :
