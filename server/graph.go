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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"code.google.com/p/plotinum/plot"
	"code.google.com/p/plotinum/plotter"
	"code.google.com/p/plotinum/plotutil"
	"code.google.com/p/plotinum/vg"
	"code.google.com/p/plotinum/vg/vgsvg"
	"github.com/sysdb/go/proto"
	"github.com/sysdb/go/sysdb"
)

func (s *Server) graph(w http.ResponseWriter, req request) {
	if len(req.args) < 2 {
		s.internal(w, fmt.Errorf("Missing host/metric information"))
	}

	host := proto.EscapeString(req.args[0])
	metric := proto.EscapeString(strings.Join(req.args[1:], "/"))
	res, err := s.query(fmt.Sprintf("TIMESERIES %s.%s", host, metric))
	if err != nil {
		s.internal(w, fmt.Errorf("Failed to retrieve graph data: %v", err))
		return
	}

	ts, ok := res.(sysdb.Timeseries)
	if !ok {
		s.internal(w, errors.New("TIMESERIES did not return a time-series"))
		return
	}

	p, err := plot.New()
	if err != nil {
		s.internal(w, fmt.Errorf("Failed to create plot: %v", err))
		return
	}
	p.Add(plotter.NewGrid())

	var i int
	for name, data := range ts.Data {
		pts := make(plotter.XYs, len(data))
		for i, p := range data {
			pts[i].X = float64(time.Time(p.Timestamp).UnixNano())
			pts[i].Y = p.Value
		}
		l, err := plotter.NewLine(pts)
		if err != nil {
			s.internal(w, fmt.Errorf("Failed to create line plotter: %v", err))
			return
		}
		l.LineStyle.Color = plotutil.DarkColors[i%len(plotutil.DarkColors)]
		p.Add(l)
		p.Legend.Add(name, l)
		i++
	}

	c := vgsvg.New(vg.Length(500), vg.Length(200))
	p.Draw(plot.MakeDrawArea(c))

	var buf bytes.Buffer
	if _, err := c.WriteTo(&buf); err != nil {
		s.internal(w, fmt.Errorf("Failed to write plot: %v", err))
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, &buf)
}

// vim: set tw=78 sw=4 sw=4 noexpandtab :
