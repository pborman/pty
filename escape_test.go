//   Copyright 2023 Paul Borman
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package main

import "testing"

func TestEscapeBuffer(t *testing.T) {
	for _, tt := range []struct {
		name  string
		size  int
		input []string
		seqs  []string
		out   string
		count int
	}{
		{
			name:  "simple",
			input: []string{"abcdefg"},
			out:   "abcdefg",
		},
		{
			name:  "byte by byte",
			input: []string{"a", "b", "c", "d", "e", "f", "g"},
			out:   "abcdefg",
		},
		{
			name:  "overflow",
			size:  4,
			input: []string{"abcdefg"},
			out:   "defg",
		},
		{
			name:  "byte by byte overflow",
			size:  4,
			input: []string{"a", "b", "c", "d", "e", "f", "g"},
			out:   "efg",
		},
		{
			name:  "one sequence",
			seqs:  []string{"xyz"},
			input: []string{"abcxyzdef"},
			count: 1,
			out:   "abcxyzdef",
		},
		{
			name:  "sequence x 3",
			seqs:  []string{"xyz"},
			input: []string{"abcxyzxyzdxyzef"},
			count: 3,
			out:   "abcxyzxyzdxyzef",
		},
		{
			name:  "sequence x 3, split",
			seqs:  []string{"xyz"},
			input: []string{"abcx", "y", "zxy", "zdxyzef"},
			count: 3,
			out:   "abcxyzxyzdxyzef",
		},
		{
			name:  "2 sequences simple",
			seqs:  []string{"xyw", "xyzzy"},
			input: []string{"axywaxyzzyx"},
			count: 2,
			out:   "axywaxyzzyx",
		},
		{
			name:  "2 sequences complex",
			seqs:  []string{"xyw", "xyzzy"},
			input: []string{"axy", "wax", "yz", "z", "yx"},
			count: 2,
			out:   "axywaxyzzyx",
		},
	} {
		if tt.size == 0 {
			tt.size = 64
		}
		e := NewEscapeBuffer(tt.size)
		count := 0
		for _, seq := range tt.seqs {
			e.AddSequence(seq, func(e *EscapeBuffer) bool {
				count++
				return true
			})
		}

		t.Run(tt.name, func(t *testing.T) {
			tlog = func(format string, v ...interface{}) {
				t.Logf(format, v...)
			}
			for _, in := range tt.input {
				e.Write([]byte(in))
			}
			e.Flush()
			if tt.out != string(e.normal) {
				t.Errorf("got %q, want %q", e.normal, tt.out)
			}
			if tt.count != count {
				t.Errorf("got count of %d, want %d", count, tt.count)
			}
		})
	}
}
