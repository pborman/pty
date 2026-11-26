//   Copyright 2026 Paul Borman
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

import (
	"errors"
	"strings"
	"testing"
)

// scanAll feeds reads to e, as the client's stdin loop does, and returns
// what is sent to the shell, with each command found shown as <.> or <:>
// where it was found.  The bytes after a command are scanned next, as they
// are after the client runs it.
func scanAll(e *escapeKey, reads ...string) string {
	var got strings.Builder
	for _, r := range reads {
		for data := []byte(r); ; {
			out, cmd, rest := e.Scan(data)
			got.Write(out)
			if cmd == 0 {
				break
			}
			got.WriteString("<" + string(cmd) + ">")
			data = rest
		}
	}
	return got.String()
}

func TestEscapeKeyScan(t *testing.T) {
	const P = "\x10" // ^P
	for _, tt := range []struct {
		in, want string
	}{
		{"", ""},
		{"hello", "hello"},
		{P + ".", "<.>"},
		{P + ":", "<:>"},
		{"abc" + P + ":def", "abc<:>def"},
		{"abc" + P + ".def", "abc<.>def"},
		{"abc" + P + P + "def", "abc" + P + "def"},
		{P + P + P + ":", P + "<:>"},
		{P + P + P + P, P + P},
		{"a" + P + "xb", "a" + P + "xb"},
		{P + "x" + P + ":ls\r" + P + ".", P + "x<:>ls\r<.>"},
		{"..::", "..::"},
		{"paste" + strings.Repeat("x", 100) + P + ":cmd\rmore", "paste" + strings.Repeat("x", 100) + "<:>cmd\rmore"},
	} {
		// The whole input in one read.
		if got := scanAll(&escapeKey{key: P[0]}, tt.in); got != tt.want {
			t.Errorf("Scan(%q) = %q, want %q", tt.in, got, tt.want)
		}
		// Split into two reads at every point.
		for i := 0; i <= len(tt.in); i++ {
			if got := scanAll(&escapeKey{key: P[0]}, tt.in[:i], tt.in[i:]); got != tt.want {
				t.Errorf("Scan(%q, %q) = %q, want %q", tt.in[:i], tt.in[i:], got, tt.want)
			}
		}
		// One byte at a time.
		var reads []string
		for i := range tt.in {
			reads = append(reads, tt.in[i:i+1])
		}
		if got := scanAll(&escapeKey{key: P[0]}, reads...); got != tt.want {
			t.Errorf("Scan(%q) a byte at a time = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEscapeKeyHeld(t *testing.T) {
	e := &escapeKey{key: 0x10}
	// A read ending with the escape character sends nothing of it yet.
	if out, cmd, _ := e.Scan([]byte("ab\x10")); string(out) != "ab" || cmd != 0 {
		t.Errorf("Scan = %q, %q", out, cmd)
	}
	// An empty read does not release it.
	if out, cmd, _ := e.Scan(nil); len(out) != 0 || cmd != 0 {
		t.Errorf("Scan(nil) = %q, %q", out, cmd)
	}
	if out, cmd, rest := e.Scan([]byte(":x")); len(out) != 0 || cmd != ':' || string(rest) != "x" {
		t.Errorf("Scan = %q, %q, %q", out, cmd, rest)
	}
}

func TestEscapeKeyNone(t *testing.T) {
	e := &escapeKey{}
	in := "a\x10.b\x10:c"
	if out, cmd, rest := e.Scan([]byte(in)); string(out) != in || cmd != 0 || rest != nil {
		t.Errorf("Scan with no key = %q, %q, %q", out, cmd, rest)
	}
}

func TestEscapeKeyOtherKey(t *testing.T) {
	e := &escapeKey{key: ']' - '@'} // ^]
	if got := scanAll(e, "a\x10.b\x1d:c"); got != "a\x10.b<:>c" {
		t.Errorf("got %q", got)
	}
}

func TestLineFrom(t *testing.T) {
	for _, tt := range []struct {
		name, typed, read string
		readErr           error
		line, rest, echo  string
		reads             bool
		err               bool
	}{
		{name: "nothing typed", read: "list", line: "list", reads: true},
		{name: "start typed", typed: "li", read: "st ", line: "list", echo: "li", reads: true},
		{name: "whole line typed", typed: " help\rls\n", line: "help", rest: "ls\n", echo: " help\r\n"},
		{name: "newline", typed: "title x\nmore", line: "title x", rest: "more", echo: "title x\r\n"},
		{name: "empty line", typed: "\r", line: "", echo: "\r\n"},
		{name: "read error", typed: "ab", readErr: errors.New("eof"), line: "ab", echo: "ab", reads: true, err: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var echo strings.Builder
			reads := false
			line, rest, err := lineFrom([]byte(tt.typed),
				func(b []byte) { echo.Write(b) },
				func() (string, error) { reads = true; return tt.read, tt.readErr })
			if line != tt.line || string(rest) != tt.rest || (err != nil) != tt.err {
				t.Errorf("lineFrom = %q, %q, %v; want %q, %q, error %v", line, rest, err, tt.line, tt.rest, tt.err)
			}
			if echo.String() != tt.echo {
				t.Errorf("echoed %q, want %q", echo.String(), tt.echo)
			}
			if reads != tt.reads {
				t.Errorf("read called: %v, want %v", reads, tt.reads)
			}
		})
	}
}
