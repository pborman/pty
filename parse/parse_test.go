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

package parse

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tt := range []struct {
		in  string
		out []string
	}{
		{},
		{in: "abc", out: []string{"abc"}},
		{in: "  abc", out: []string{"abc"}},
		{in: "abc  ", out: []string{"abc"}},
		{in: `" abc "`, out: []string{" abc "}},
		{in: "  abc  ", out: []string{"abc"}},
		{in: "abc def", out: []string{"abc", "def"}},
		{in: "abc   def", out: []string{"abc", "def"}},
		{in: "abc;def", out: []string{"abc", ";", "def"}},
		{in: "abc; def", out: []string{"abc", ";", "def"}},
		{in: "abc ; def", out: []string{"abc", ";", "def"}},
		{in: "abc|def", out: []string{"abc", "|", "def"}},
	} {
		out, err := Line(tt.in)
		if err != nil {
			t.Errorf("%s: %v", tt.in, err)
		}
		if !reflect.DeepEqual(out, tt.out) {
			t.Errorf("%s: got %q, want %q", tt.in, out, tt.out)
		}
	}
}
