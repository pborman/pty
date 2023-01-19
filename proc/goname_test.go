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

package proc

import (
	"testing"
)

var gonametests = []struct {
	in  string
	out string
}{
	{"$foo_bar", "FooBar"}, // make sure the doc string is right!
	{"abc", "Abc"},
	{" a b c ", "ABC"},
	{"$abc", "Abc"},
	{"-abc", "Abc"},
	{".abc", "Abc"},
	{"Abc", "Abc"},
	{"Abc_", "Abc"},
	{"Ab_c", "AbC"},
	{"Ab(c)", "AbC"},
	{"A_b_c", "ABC"},
	{"1abc", "X1Abc"},
}

func TestGoName(t *testing.T) {
	for _, tt := range gonametests {
		out := GoName(tt.in)
		if out != tt.out {
			t.Errorf("%q: got %q, want %q", tt.in, out, tt.out)
		}
	}
}
