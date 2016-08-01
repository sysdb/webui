//
// Copyright (C) 2016 Sebastian 'tokkee' Harl <sh@tokkee.org>
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

package graph

import (
	"reflect"
	"testing"
	"time"

	"github.com/sysdb/go/sysdb"
)

func TestAlign(t *testing.T) {
	for _, test := range []struct {
		ts1, ts2 *sysdb.Timeseries
		want     *sysdb.Timeseries
	}{
		{
			ts1: &sysdb.Timeseries{
				Start: ts(4, 5, 0),
				End:   ts(4, 10, 0),
				Data: map[string][]sysdb.DataPoint{
					"value": []sysdb.DataPoint{
						{ts(4, 5, 0), 0.0},
						{ts(4, 6, 0), 0.0},
						{ts(4, 7, 0), 1.0},
						{ts(4, 8, 0), 1.0},
						{ts(4, 9, 0), 0.0},
						{ts(4, 10, 0), 0.0},
					},
				},
			},
			ts2: &sysdb.Timeseries{
				Start: ts(4, 7, 0),
				End:   ts(4, 8, 0),
				Data: map[string][]sysdb.DataPoint{
					"value": []sysdb.DataPoint{
						{ts(4, 7, 0), 1.0},
						{ts(4, 8, 0), 1.0},
					},
				},
			},
			want: &sysdb.Timeseries{
				Start: ts(4, 7, 0),
				End:   ts(4, 8, 0),
				Data: map[string][]sysdb.DataPoint{
					"value": []sysdb.DataPoint{
						{ts(4, 7, 0), 1.0},
						{ts(4, 8, 0), 1.0},
					},
				},
			},
		},
		{
			ts1: &sysdb.Timeseries{
				Start: ts(4, 7, 0),
				End:   ts(4, 8, 0),
				Data: map[string][]sysdb.DataPoint{
					"value": []sysdb.DataPoint{
						{ts(4, 7, 0), 1.0},
						{ts(4, 8, 0), 1.0},
					},
				},
			},
			ts2: &sysdb.Timeseries{
				Start: ts(4, 7, 0),
				End:   ts(4, 8, 0),
				Data: map[string][]sysdb.DataPoint{
					"value": []sysdb.DataPoint{
						{ts(4, 7, 0), 1.0},
						{ts(4, 8, 0), 1.0},
					},
				},
			},
			want: &sysdb.Timeseries{
				Start: ts(4, 7, 0),
				End:   ts(4, 8, 0),
				Data: map[string][]sysdb.DataPoint{
					"value": []sysdb.DataPoint{
						{ts(4, 7, 0), 1.0},
						{ts(4, 8, 0), 1.0},
					},
				},
			},
		},
		{
			ts1: &sysdb.Timeseries{
				Start: ts(4, 5, 0),
				End:   ts(4, 10, 0),
				Data: map[string][]sysdb.DataPoint{
					"value": []sysdb.DataPoint{
						{ts(4, 5, 0), 0.0},
						{ts(4, 6, 0), 0.0},
						{ts(4, 7, 0), 0.0},
						{ts(4, 8, 0), 1.0},
						{ts(4, 9, 0), 1.0},
						{ts(4, 10, 0), 1.0},
					},
				},
			},
			ts2: &sysdb.Timeseries{
				Start: ts(4, 8, 0),
				End:   ts(4, 12, 0),
				Data: map[string][]sysdb.DataPoint{
					"value": []sysdb.DataPoint{
						{ts(4, 8, 0), 1.0},
						{ts(4, 9, 0), 1.0},
						{ts(4, 10, 0), 1.0},
						{ts(4, 11, 0), 0.0},
						{ts(4, 12, 0), 0.0},
					},
				},
			},
			want: &sysdb.Timeseries{
				Start: ts(4, 8, 0),
				End:   ts(4, 10, 0),
				Data: map[string][]sysdb.DataPoint{
					"value": []sysdb.DataPoint{
						{ts(4, 8, 0), 1.0},
						{ts(4, 9, 0), 1.0},
						{ts(4, 10, 0), 1.0},
					},
				},
			},
		},
		{
			ts1: &sysdb.Timeseries{
				Start: ts(4, 7, 0),
				End:   ts(4, 7, 0),
				Data: map[string][]sysdb.DataPoint{
					"value": []sysdb.DataPoint{
						{ts(4, 7, 0), 1.0},
					},
				},
			},
			ts2: &sysdb.Timeseries{
				Start: ts(4, 7, 0),
				End:   ts(4, 7, 0),
				Data: map[string][]sysdb.DataPoint{
					"value": []sysdb.DataPoint{
						{ts(4, 7, 0), 1.0},
					},
				},
			},
			want: &sysdb.Timeseries{
				Start: ts(4, 7, 0),
				End:   ts(4, 7, 0),
				Data: map[string][]sysdb.DataPoint{
					"value": []sysdb.DataPoint{
						{ts(4, 7, 0), 1.0},
					},
				},
			},
		},
	} {
		if err := align(test.ts1, test.ts2); err != nil {
			t.Errorf("align(%v, %v) = %v; want <nil>", test.ts1, test.ts2, err)
		}

		if !reflect.DeepEqual(test.ts1, test.want) || !reflect.DeepEqual(test.ts2, test.want) {
			t.Errorf("align() unexpected result %v, %v; want %v", test.ts1, test.ts2, test.want)
		}
	}
}

func ts(hour, min, sec int) sysdb.Time {
	return sysdb.Time(time.Date(2016, 1, 1, hour, min, sec, 0, time.UTC))
}

// vim: set tw=78 sw=4 sw=4 noexpandtab :
