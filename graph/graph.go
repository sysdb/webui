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
	"sort"
	"strings"
	"time"

	"github.com/gonum/plot"
	"github.com/gonum/plot/plotter"
	"github.com/gonum/plot/plotutil"
	"github.com/sysdb/go/client"
	"github.com/sysdb/go/sysdb"
)

// A Metric represents a single data-source of a graph.
type Metric struct {
	// The unique identifier of the metric.
	Hostname, Identifier string

	// Attributes describing details of the metric.
	Attributes map[string]string

	ts *sysdb.Timeseries
}

// A Graph represents a single graph. It may reference multiple data-sources.
type Graph struct {
	// Time range of the graph.
	Start, End time.Time

	// Content of the graph.
	Metrics []Metric

	// List of attributes to group by.
	GroupBy []string
}

type pl struct {
	*plot.Plot

	ts int // Index of the current time-series.
}

func queryTimeseries(c *client.Client, metric Metric, start, end time.Time) (*sysdb.Timeseries, error) {
	q, err := client.QueryString("TIMESERIES %s.%s START %s END %s",
		metric.Hostname, metric.Identifier, start, end)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve graph data: %v", err)
	}
	res, err := c.Query(q)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve graph data: %v", err)
	}

	ts, ok := res.(*sysdb.Timeseries)
	if !ok {
		return nil, fmt.Errorf("TIMESERIES did not return a time-series but %T", res)
	}
	return ts, nil
}

func (p *pl) addTimeseries(c *client.Client, metric Metric, verbose bool) error {
	for name, data := range metric.ts.Data {
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
		if verbose {
			p.Legend.Add(fmt.Sprintf("%s %s %s", metric.Hostname, metric.Identifier, name), l)
		} else {
			p.Legend.Add(name, l)
		}
		p.ts++
	}
	return nil
}

// align aligns two timeseries such that start and end times and the step
// sizes match.
func align(ts1, ts2 *sysdb.Timeseries) error {
	if len(ts1.Data) != len(ts2.Data) {
		return fmt.Errorf("mismatching data sources: %v != %v", ts1.Data, ts2.Data)
	}

	start := time.Time(ts1.Start)
	if t := time.Time(ts2.Start); t.After(start) {
		start = t
	}
	end := time.Time(ts1.End)
	if t := time.Time(ts2.End); t.Before(end) {
		end = t
	}
	if end.Before(start) {
		return fmt.Errorf("non-overlapping ranges: [%v, %v] <-> [%v, %v]",
			ts1.Start, ts1.End, ts2.Start, ts2.End)
	}

	range1 := time.Time(ts1.End).Sub(time.Time(ts1.Start))
	range2 := time.Time(ts2.End).Sub(time.Time(ts2.Start))
	for name := range ts1.Data {
		l1, l2 := len(ts1.Data[name]), len(ts2.Data[name])
		if l1 <= 1 || l2 <= 1 {
			if l1 == l2 && range1 == range2 {
				continue
			}
			return fmt.Errorf("invalid value count for %q: %d != %d", name, l1, l2)
		}

		step1, step2 := range1/time.Duration(l1-1), range2/time.Duration(l2-1)
		if step1 != step2 || step1 <= 0 {
			return fmt.Errorf("mismatching steps sizes for %q: %v != %v", name, step1, step2)
		}

		for _, ts := range []*sysdb.Timeseries{ts1, ts2} {
			a := start.Sub(time.Time(ts.Start)) / step1
			b := end.Sub(time.Time(ts.Start)) / step1
			ts.Data[name] = ts.Data[name][a : b+1]
		}
	}

	ts1.Start, ts2.Start = sysdb.Time(start), sysdb.Time(start)
	ts1.End, ts2.End = sysdb.Time(end), sysdb.Time(end)
	return nil
}

// sum is an aggregation function that adds ts2 to ts1. The timeseries will be
// aligned.
func sum(ts1, ts2 *sysdb.Timeseries) error {
	if err := align(ts1, ts2); err != nil {
		return fmt.Errorf("Incompatible time-series: %v", err)
	}

	for name := range ts1.Data {
		for i := range ts1.Data[name] {
			ts1.Data[name][i].Value += ts2.Data[name][i].Value
		}
	}
	return nil
}

func (g *Graph) group(c *client.Client, start, end time.Time) ([]Metric, error) {
	if len(g.GroupBy) == 0 {
		for i, m := range g.Metrics {
			var err error
			if g.Metrics[i].ts, err = queryTimeseries(c, m, g.Start, g.End); err != nil {
				return nil, err
			}
		}
		return g.Metrics, nil
	}

	names := make([]string, 0)
	groups := make(map[string][]Metric)
	for _, m := range g.Metrics {
		var key string
		for _, g := range g.GroupBy {
			key += "\x00" + m.Attributes[g]
		}
		if _, ok := groups[key]; !ok {
			names = append(names, key)
		}
		groups[key] = append(groups[key], m)
	}
	sort.Strings(names)

	var metrics []Metric
	for _, name := range names {
		group := groups[name]
		ts, err := queryTimeseries(c, group[0], g.Start, g.End)
		if err != nil {
			return nil, err
		}
		host := group[0].Hostname
		for _, m := range group[1:] {
			ts2, err := queryTimeseries(c, m, g.Start, g.End)
			if err != nil {
				return nil, err
			}
			if err := sum(ts, ts2); err != nil {
				return nil, err
			}
			if host != "" && host != m.Hostname {
				host = ""
			}
		}

		metrics = append(metrics, Metric{
			Hostname:   host,
			Identifier: strings.Replace(name[1:], "\x00", "-", -1),
			ts:         ts,
		})
	}
	return metrics, nil
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

	metrics, err := g.group(c, g.Start, g.End)
	if err != nil {
		return nil, err
	}
	for _, m := range metrics {
		if err := p.addTimeseries(c, m, len(g.Metrics) > 1); err != nil {
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
