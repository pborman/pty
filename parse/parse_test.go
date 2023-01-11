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
		{ in:  "abc", out: []string{"abc"} },
		{ in:  "  abc", out: []string{"abc"} },
		{ in:  "abc  ", out: []string{"abc"} },
		{ in:  `" abc "`, out: []string{" abc "} },
		{ in:  "  abc  ", out: []string{"abc"} },
		{ in:  "abc def", out: []string{"abc", "def"}, },
		{ in:  "abc   def", out: []string{"abc", "def"}, },
		{ in:  "abc;def", out: []string{"abc", ";", "def"}, },
		{ in:  "abc; def", out: []string{"abc", ";", "def"}, },
		{ in:  "abc ; def", out: []string{"abc", ";", "def"}, },
		{ in:  "abc|def", out: []string{"abc", "|", "def"}, },
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
