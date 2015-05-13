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

// Helper functions for handling and plotting graphs.

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gonum/plot/vg"
	"github.com/sysdb/webui/graph"
)

var urldate = "20060102150405"

func (s *Server) graph(w http.ResponseWriter, req request) {
	if len(req.args) < 2 || 4 < len(req.args) {
		s.badrequest(w, fmt.Errorf("Missing host/metric information"))
		return
	}

	end := time.Now()
	start := end.Add(-24 * time.Hour)
	var err error
	if len(req.args) > 2 {
		if start, err = time.Parse(urldate, req.args[2]); err != nil {
			s.badrequest(w, fmt.Errorf("Invalid start time: %v", err))
			return
		}
	}
	if len(req.args) > 3 {
		if end, err = time.Parse(urldate, req.args[3]); err != nil {
			s.badrequest(w, fmt.Errorf("Invalid start time: %v", err))
			return
		}
	}
	g := &graph.Graph{
		Start:   start,
		End:     end,
		Metrics: []graph.Metric{{Hostname: req.args[0], Identifier: req.args[1]}},
	}
	p, err := g.Plot(s.c)
	if err != nil {
		s.internal(w, err)
		return
	}

	pw, err := p.WriterTo(vg.Length(500), vg.Length(200), "svg")
	if err != nil {
		s.internal(w, fmt.Errorf("Failed to write plot: %v", err))
		return
	}

	var buf bytes.Buffer
	if _, err := pw.WriteTo(&buf); err != nil {
		s.internal(w, fmt.Errorf("Failed to write plot: %v", err))
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, &buf)
}

// vim: set tw=78 sw=4 sw=4 noexpandtab :
