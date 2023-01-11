// Copyright 2011 Google Inc. All rights reserved.
// Author: borman@google.com (Paul Borman)

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
