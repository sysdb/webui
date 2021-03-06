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
	"strings"
	"time"

	"github.com/gonum/plot/vg"
	"github.com/sysdb/go/sysdb"
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
		Start: start,
		End:   end,
	}
	if req.args[0] == "q" || len(req.args[0]) > 1 && req.args[0][:2] == "q/" {
		if g.Metrics, err = s.queryMetrics(req.args[1]); err != nil {
			s.badrequest(w, fmt.Errorf("Failed to query metrics: %v", err))
			return
		}

		if req.args[0] != "q" {
			for _, arg := range strings.Split(req.args[0][2:], "/") {
				if arg := strings.SplitN(arg, "=", 2); len(arg) == 2 {
					if arg[0] == "g" {
						g.GroupBy = strings.Split(arg[1], ",")
					}
				}
			}
		}
	} else {
		g.Metrics = []graph.Metric{{Hostname: req.args[0], Identifier: req.args[1]}}
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

func (s *Server) queryMetrics(q string) ([]graph.Metric, error) {
	raw, err := parseQuery(q)
	if err != nil {
		return nil, err
	}
	if raw.typ != "" && raw.typ != "metrics" {
		return nil, fmt.Errorf("Invalid object type %q for graphs", raw.typ)
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

	res, err := s.c.Query("LOOKUP metrics MATCHING" + args)
	if err != nil {
		return nil, err
	}
	hosts, ok := res.([]sysdb.Host)
	if !ok {
		return nil, fmt.Errorf("LOOKUP did not return a list of hosts but %T", res)
	}
	var metrics []graph.Metric
	for _, h := range hosts {
		for _, m := range h.Metrics {
			metric := graph.Metric{
				Hostname:   h.Name,
				Identifier: m.Name,
				Attributes: make(map[string]string),
			}
			for _, attr := range m.Attributes {
				metric.Attributes[attr.Name] = attr.Value
			}
			metrics = append(metrics, metric)
		}
	}
	return metrics, nil
}

// vim: set tw=78 sw=4 sw=4 noexpandtab :
