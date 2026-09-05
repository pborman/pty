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

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"
)

func TestParseEscapeChar(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want byte
		ok   bool
	}{
		{"", 0, true},
		{"a", 'a', true},
		{"^P", 'P' & 037, true},
		{"^A", 1, true},
		{`\0`, 0, true},
		{`\n`, '\n', true},
		{`\176`, '~', true},
		{`"\t"`, '\t', true},
		{"ab", 0, false},
		{"abc", 0, false},
		{"^", '^', true},
		{`"xy"`, 0, false},
	} {
		got, ok := parseEscapeChar(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseEscapeChar(%q) = %d, %v; want %d, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestPrintEscape(t *testing.T) {
	for _, tt := range []struct {
		in   byte
		want string
	}{
		{0, "^@"},
		{1, "^A"},
		{'P' & 037, "^P"},
		{'A', "A"},
		{'~', "~"},
		{0x7f, `\x7f`},
		{0x80, `\u0080`},
	} {
		if got := printEscape(tt.in); got != tt.want {
			t.Errorf("printEscape(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEncodeDecodeSize(t *testing.T) {
	for _, tt := range []struct{ rows, cols int }{
		{0, 0},
		{24, 80},
		{1, 1},
		{255, 255},
		{256, 512},
		{65535, 65535},
	} {
		buf := encodeSize(tt.rows, tt.cols)
		if len(buf) != 4 {
			t.Fatalf("encodeSize(%d,%d) len = %d", tt.rows, tt.cols, len(buf))
		}
		rows, cols := decodeSize(buf)
		if rows != tt.rows || cols != tt.cols {
			t.Errorf("roundtrip %d x %d -> %d x %d", tt.rows, tt.cols, rows, cols)
		}
	}
}

func TestQuoteShell(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"", `""`},
		{"hello", `"hello"`},
		{`a"b`, `"a\"b"`},
		{`a\b`, `"a\\b"`},
		{"a$b", `"a\$b"`},
		{"a`b", "\"a\\`b\""},
	} {
		if got := quoteShell(tt.in); got != tt.want {
			t.Errorf("quoteShell(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestSplur(t *testing.T) {
	if splur(1) != "" {
		t.Errorf("splur(1) = %q, want empty", splur(1))
	}
	if splur(0) != "s" {
		t.Errorf("splur(0) = %q, want s", splur(0))
	}
	if splur(2) != "s" {
		t.Errorf("splur(2) = %q, want s", splur(2))
	}
}

func TestPrintfWarnfDebugf(t *testing.T) {
	printf("hello printf %d", 1)
	printf("already has newline\n")
	warnf("hello warn %s", "x")

	if debugLog != nil {
		t.Fatal("debugLog already set")
	}
	debugf("no-op while unset")

	path := filepath.Join(t.TempDir(), "debug.log")
	debugInit(path)
	defer func() {
		if debugLog != nil {
			debugLog.Close()
			debugLog = nil
		}
	}()
	debugf("debug line")
	debugf("debug with newline\n")
	debugLog.Sync()
	data, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("debug line")) {
		t.Errorf("debug log = %q, want debug line", data)
	}
}

func TestIsPipe(t *testing.T) {
	_ = isPipe()
}
