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
	"bytes"
	"encoding/base64"
	"io/ioutil"
	"testing"
)

func clearSeq(screen, st string) string {
	return clearIntro + base64.StdEncoding.EncodeToString([]byte(screen)) + st
}

// filterAll runs chunks through one clearFilter and returns the input that
// passed through and the screens that were cleared, rendered as strings.
func filterAll(chunks ...string) (string, []string) {
	var f clearFilter
	var out []byte
	var screens []string
	for _, c := range chunks {
		out = append(out, f.Filter([]byte(c), func(b []byte) {
			screens = append(screens, string(b))
		})...)
	}
	return string(out), screens
}

func TestClearFilter(t *testing.T) {
	screen := "\033[1m$ \033[0mls\033[1;5H"
	for _, tt := range []struct {
		name    string
		chunks  []string
		out     string
		screens []string
	}{
		{"plain", []string{"hello"}, "hello", nil},
		{"ST", []string{"ab" + clearSeq(screen, "\033\\") + "cd"}, "abcd", []string{screen}},
		{"BEL", []string{"ab" + clearSeq(screen, "\a") + "cd"}, "abcd", []string{screen}},
		{"empty", []string{clearSeq("", "\033\\")}, "", []string{""}},
		{"two", []string{clearSeq("a", "\a") + "x" + clearSeq("b", "\033\\")}, "x", []string{"a", "b"}},
		{"lone ESC", []string{"\033"}, "\033", nil},
		{"ESC then key", []string{"\033", "x"}, "\033x", nil},
		{"arrow", []string{"\033[A"}, "\033[A", nil},
		{"alt-]", []string{"\033]"}, "", nil}, // held: may be a split sequence
		{"alt-] then key", []string{"\033]", "x"}, "\033]x", nil},
		{"other OSC", []string{"\033]7717;ropen\a"}, "\033]7717;ropen\a", nil},
		{"false start held", []string{"\033]77", "x"}, "\033]77x", nil},
		{"false start then real", []string{"\033]7" + clearSeq("s", "\a")}, "\033]7", []string{"s"}},
		{"bad padding", []string{clearSeq("", "") + "abc\a"}, "", nil},
		{"cut off by CR", []string{clearSeq("abc", "") + "\r"}, "\r", nil},
		{"cut off by arrow", []string{clearSeq("abc", "") + "\033[A"}, "\033[A", nil},
		{"cut off by new clear", []string{clearSeq("abc", "") + clearSeq("s", "\a")}, "", []string{"s"}},
		{"ST split", []string{clearSeq(screen, "\033"), "\\z"}, "z", []string{screen}},
	} {
		out, screens := filterAll(tt.chunks...)
		if out != tt.out {
			t.Errorf("%s: input %q, want %q", tt.name, out, tt.out)
		}
		if len(screens) != len(tt.screens) {
			t.Errorf("%s: screens %q, want %q", tt.name, screens, tt.screens)
			continue
		}
		for i := range screens {
			if screens[i] != tt.screens[i] {
				t.Errorf("%s: screen %d %q, want %q", tt.name, i, screens[i], tt.screens[i])
			}
		}
	}
}

// A sequence split anywhere after its first byte is still recognized.
// (A split right after the leading ESC is not: a lone ESC at the end of a
// read is passed on at once as the escape key.)
func TestClearFilterSplit(t *testing.T) {
	in := "ab" + clearSeq("$ ls\033[1;5H", "\033\\") + "cd"
	for i := len("ab\033") + 1; i < len(in); i++ {
		out, screens := filterAll(in[:i], in[i:])
		if out != "abcd" || len(screens) != 1 || screens[0] != "$ ls\033[1;5H" {
			t.Errorf("split at %d: input %q, screens %q", i, out, screens)
		}
	}
}

// clientOutput stops c and returns the data it was sent.
func clientOutput(t *testing.T, c *Client, buf *bytes.Buffer) string {
	t.Helper()
	stopClient(c)
	data, err := ioutil.ReadAll(NewMessengerReader(buf, nil))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestShellClearScreen(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("clearsess", "")
	defer sess.Remove()
	s := NewShell(sess)
	s.eb.Write([]byte("old history\r\n$ "))

	var buf bytes.Buffer
	c := NewClient(&buf)
	s.ClearScreen(c, []byte("$ \033[1;3H"))

	want := clearPrefix + "$ \033[1;3H"
	if got := string(s.eb.normal); got != want {
		t.Errorf("recorded %q, want %q", got, want)
	}
	if got := clientOutput(t, c, &buf); got != edsaved+want {
		t.Errorf("sent to client %q, want %q", got, edsaved+want)
	}

	// Later output is recorded after the new screen.
	s.eb.Write([]byte("more"))
	if got := string(s.eb.normal); got != want+"more" {
		t.Errorf("after more output, recorded %q", got)
	}
}

func TestShellClearScreenAlt(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("clearalt", "")
	defer sess.Remove()
	s := NewShell(sess)
	s.eb.Write([]byte("history" + scasb + "vi screen"))

	var buf bytes.Buffer
	c := NewClient(&buf)
	s.ClearScreen(c, []byte("$ "))

	if got := string(s.eb.normal); got != "history" {
		t.Errorf("normal record changed to %q", got)
	}
	if got := string(s.eb.alt); got != "vi screen" {
		t.Errorf("alt record changed to %q", got)
	}
	if got := clientOutput(t, c, &buf); got != "" {
		t.Errorf("sent to client %q, want nothing", got)
	}
}
