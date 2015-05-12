//
// Copyright (C) 2014-2015 Sebastian 'tokkee' Harl <sh@tokkee.org>
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

// Package graph handles time-series data provided by SysDB. It supports
// querying and post-processing of the data.
package graph

import (
	"fmt"
	"time"

	"github.com/gonum/plot"
	"github.com/gonum/plot/plotter"
	"github.com/gonum/plot/plotutil"
	"github.com/sysdb/go/client"
	"github.com/sysdb/go/sysdb"
)

// A Graph represents a single graph. It may reference multiple time-series.
type Graph struct {
	// Time range of the graph.
	Start, End time.Time

	// Metrics: {<hostname>, <identifier>}
	Metrics [][2]string
}

type pl struct {
	*plot.Plot

	ts int // Index of the current time-series.
}

func (p *pl) addTimeseries(c *client.Client, metric [2]string, start, end time.Time) error {
	q, err := client.QueryString("TIMESERIES %s.%s START %s END %s", metric[0], metric[1], start, end)
	if err != nil {
		return fmt.Errorf("Failed to retrieve graph data: %v", err)
	}
	res, err := c.Query(q)
	if err != nil {
		return fmt.Errorf("Failed to retrieve graph data: %v", err)
	}

	ts, ok := res.(*sysdb.Timeseries)
	if !ok {
		return fmt.Errorf("TIMESERIES did not return a time-series but %T", res)
	}

	for name, data := range ts.Data {
		pts := make(plotter.XYs, len(data))
		for i, p := range data {
			pts[i].X = float64(time.Time(p.Timestamp).UnixNano())
			pts[i].Y = p.Value
		}

		l, err := plotter.NewLine(pts)
		if err != nil {
			return fmt.Errorf("Failed to create line plotter: %v", err)
		}
		l.LineStyle.Color = plotutil.DarkColors[p.ts%len(plotutil.DarkColors)]

		p.Add(l)
		p.Legend.Add(name, l)
		p.ts++
	}
	return nil
}

// Plot fetches a graph's time-series data using the specified client and
// plots it.
func (g *Graph) Plot(c *client.Client) (*plot.Plot, error) {
	var err error

	p := &pl{}
	p.Plot, err = plot.New()
	if err != nil {
		return nil, fmt.Errorf("Failed to create plot: %v", err)
	}
	p.Add(plotter.NewGrid())
	p.X.Tick.Marker = dateTicks{}

	for _, m := range g.Metrics {
		if err := p.addTimeseries(c, m, g.Start, g.End); err != nil {
			return nil, err
		}
	}
	return p.Plot, nil
}

type dateTicks struct{}

func (dateTicks) Ticks(min, max float64) []plot.Tick {
	// TODO: this is surely not the best we can do
	// but it'll distribute ticks evenly.
	ticks := plot.DefaultTicks{}.Ticks(min, max)
	for i, t := range ticks {
		if t.Label == "" {
			// Skip minor ticks.
			continue
		}
		ticks[i].Label = time.Unix(0, int64(t.Value)).Format(time.RFC822)
	}
	return ticks
}

// vim: set tw=78 sw=4 sw=4 noexpandtab :
