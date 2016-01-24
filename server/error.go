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

// Error handlers for the SysDB web interface.

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
)

func (s *Server) notfound(w http.ResponseWriter, r *http.Request) {
	s.err(w, http.StatusNotFound, fmt.Errorf("%s not found", r.URL.Path))
}

func (s *Server) badrequest(w http.ResponseWriter, err error) {
	s.err(w, http.StatusBadRequest, err)
}

func (s *Server) internal(w http.ResponseWriter, err error) {
	s.err(w, http.StatusInternalServerError, err)
}

func (s *Server) err(w http.ResponseWriter, status int, err error) {
	log.Printf("%s: %v", http.StatusText(status), err)

	page := page{
		Title:   "SysDB - Error",
		Content: "<section class=\"error\">" + html(err.Error()) + "</section>",
	}

	var buf bytes.Buffer
	err = s.main.Execute(&buf, &page)
	if err != nil {
		// Nothing more we can do about this.
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(status)
	io.Copy(w, &buf)
}

// vim: set tw=78 sw=4 sw=4 noexpandtab :
