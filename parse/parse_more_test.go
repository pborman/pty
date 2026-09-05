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
	"bytes"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseMore(t *testing.T) {
	for _, tt := range []struct {
		in      string
		out     []string
		wantErr bool
	}{
		{in: `"hello world"`, out: []string{"hello world"}},
		{in: `'single'`, out: []string{"single"}},
		{in: `"say hi"`, out: []string{"say hi"}},
		{in: `abc|def`, out: []string{"abc", "|", "def"}},
		{in: `abc|`, out: []string{"abc", "|"}},
		{in: `|abc`, out: []string{"|", "abc"}},
		{in: `a\ b`, out: []string{"a b"}},
		{in: "abc\ndef", out: []string{"abc"}},
		{in: "\"ab\\nc\"", out: []string{"abnc"}},
		{in: `""`, out: []string{""}},
		{in: "a\\\nb", out: []string{"ab"}},
		// An unmatched quote in a typed line is closed at EOF rather than
		// returned as an error.
		{in: `"unclosed`, out: []string{"unclosed"}},
		{in: `'unclosed`, out: []string{"unclosed"}},
		{in: `abc;`, out: []string{"abc", ";"}},
		{in: `;`, out: []string{";"}},
		{in: "   \n", out: nil},
		{in: `\`, out: []string{"\x00"}}, // trailing backslash at EOF
	} {
		t.Run(tt.in, func(t *testing.T) {
			out, err := Line(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Line(%q) = %q, want error", tt.in, out)
				}
				return
			}
			if err != nil {
				t.Errorf("Line(%q): %v (got %q)", tt.in, err, out)
				return
			}
			if !reflect.DeepEqual(out, tt.out) {
				t.Errorf("Line(%q) = %#v, want %#v", tt.in, out, tt.out)
			}
		})
	}
}

func TestReaderPS1(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	pr := NewReader(strings.NewReader("hi\n"))
	pr.PS1 = "PROMPT>"
	words, err := pr.Read()
	w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(words, []string{"hi"}) {
		t.Errorf("got %q", words)
	}
	prompt, _ := io.ReadAll(r)
	if !bytes.Contains(prompt, []byte("PROMPT>")) {
		t.Errorf("PS1 not written, got %q", prompt)
	}
}

func TestReaderQuotedNewlinePS2(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	pr := NewReader(strings.NewReader("\"ab\nc\"\n"))
	pr.PS2 = "MORE>"
	words, err := pr.Read()
	w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(words, []string{"ab\nc"}) {
		t.Errorf("got %#v", words)
	}
	prompt, _ := io.ReadAll(r)
	if !bytes.Contains(prompt, []byte("MORE>")) {
		t.Errorf("PS2 not written for quoted newline, got %q", prompt)
	}
}

func TestReaderEOF(t *testing.T) {
	pr := NewReader(strings.NewReader(""))
	words, err := pr.Read()
	if err != nil {
		t.Fatalf("empty Read: %v", err)
	}
	if words != nil {
		t.Errorf("empty Read = %q, want nil", words)
	}
}

func TestIsQuoteDelim(t *testing.T) {
	if !isQuote('"') || !isQuote('\'') || isQuote('x') {
		t.Error("isQuote mismatch")
	}
	if !isDelim('|') || !isDelim(';') || isDelim('x') {
		t.Error("isDelim mismatch")
	}
}
