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
	"strings"
	"testing"
)

func TestNewEscapeBufferDefaultSize(t *testing.T) {
	e := NewEscapeBuffer(0)
	if cap(e.normal) != 1024*1024 {
		t.Errorf("default cap = %d, want 1M", cap(e.normal))
	}
	e.AddSequence("", func(*EscapeBuffer) bool { return true })
	if len(e.sequences) != 0 {
		t.Error("empty AddSequence should be ignored")
	}
}

func TestAddSequenceFalseDropsMatch(t *testing.T) {
	e := NewEscapeBuffer(64)
	e.AddSequence("XYZ", func(*EscapeBuffer) bool { return false })
	e.Write([]byte("abXYZcd"))
	e.Flush()
	if string(e.normal) != "abcd" {
		t.Errorf("callback false: got %q, want abcd", e.normal)
	}
}

func TestAddReportSequence(t *testing.T) {
	e := NewEscapeBuffer(64)
	var seen []string
	e.AddReportSequence("\033]", "\033\\", func(_ *EscapeBuffer, data []byte) bool {
		seen = append(seen, string(data))
		return true
	})
	e.Write([]byte("pre\033]title\033\\post"))
	e.Flush()
	if len(seen) != 1 || seen[0] != "title" {
		t.Errorf("report payload = %q, want [title]", seen)
	}
	if string(e.normal) != "pretitlepost" {
		t.Errorf("buffer = %q, want pretitlepost", e.normal)
	}
}

func TestAddReportSequenceEmptyTerm(t *testing.T) {
	e := NewEscapeBuffer(64)
	called := 0
	e.AddReportSequence("XYZ", "", func(*EscapeBuffer, []byte) bool {
		called++
		return true
	})
	e.Write([]byte("abXYZcd"))
	e.Flush()
	if called != 1 {
		t.Errorf("empty term callback count = %d", called)
	}
}

func TestAddReportSequenceEmptySeq(t *testing.T) {
	e := NewEscapeBuffer(64)
	e.AddReportSequence("", "x", func(*EscapeBuffer, []byte) bool { return true })
	if len(e.sequences) != 0 {
		t.Error("empty report sequence should be ignored")
	}
}

func TestReportSequenceSplitTerminator(t *testing.T) {
	e := NewEscapeBuffer(64)
	var seen []string
	e.AddReportSequence("\033]", "\033\\", func(_ *EscapeBuffer, data []byte) bool {
		seen = append(seen, string(data))
		return false
	})
	e.Write([]byte("ab\033]hi\033"))
	e.Write([]byte("\\cd"))
	e.Flush()
	if len(seen) != 1 || seen[0] != "hi" {
		t.Errorf("split terminator: payload = %q", seen)
	}
}

func TestEscapeInaltAndSendEscapes(t *testing.T) {
	e := NewEscapeBuffer(64)
	e.AddSequence("\033[?1049h", func(eb *EscapeBuffer) bool {
		eb.inalt = true
		return false
	})
	e.AddSequence("\033[?1049l", func(eb *EscapeBuffer) bool {
		eb.inalt = false
		return false
	})
	e.Write([]byte("normal\033[?1049halt\033[31mtext\033[?1049lmore"))
	e.Flush()
	if string(e.normal) != "normalmore" {
		t.Errorf("normal = %q, want normalmore", e.normal)
	}
	if !bytes.Contains(e.alt, []byte("alt")) {
		t.Errorf("alt = %q, want to contain alt", e.alt)
	}

	var buf bytes.Buffer
	e.sendEscapes(&buf, false)
	if !bytes.Contains(buf.Bytes(), []byte("-----")) {
		t.Errorf("sendEscapes normal = %q", buf.Bytes())
	}
	buf.Reset()
	e.sendEscapes(&buf, true)
	if !bytes.Contains(buf.Bytes(), []byte("-----")) {
		t.Errorf("sendEscapes alt = %q", buf.Bytes())
	}
}

func TestEscapeWriteNoSequences(t *testing.T) {
	e := NewEscapeBuffer(8)
	n, err := e.Write([]byte("hello"))
	if n != 5 || err != nil {
		t.Errorf("Write = %d, %v", n, err)
	}
	e.Flush()
	if string(e.normal) != "hello" {
		t.Errorf("got %q", e.normal)
	}
}

func TestEscapePartialThenComplete(t *testing.T) {
	e := NewEscapeBuffer(64)
	count := 0
	e.AddSequence("ABCD", func(*EscapeBuffer) bool {
		count++
		return true
	})
	e.Write([]byte("xxAB"))
	if count != 0 {
		t.Error("partial should not fire")
	}
	e.Write([]byte("CDyy"))
	e.Flush()
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if string(e.normal) != "xxABCDyy" {
		t.Errorf("got %q", e.normal)
	}
}

func TestEscapeFirstByteNotSequence(t *testing.T) {
	e := NewEscapeBuffer(64)
	e.AddSequence("XYZ", func(*EscapeBuffer) bool { return true })
	e.Write([]byte("Xnot"))
	e.Flush()
	if string(e.normal) != "Xnot" {
		t.Errorf("got %q", e.normal)
	}
}

func TestAppendtoOverflow(t *testing.T) {
	e := NewEscapeBuffer(4)
	e.Write([]byte("abcd"))
	e.Write([]byte("efghij"))
	e.Flush()
	if len(e.normal) > 4 {
		t.Errorf("overflow kept %d bytes, cap 4", len(e.normal))
	}
}

func TestAddReportSequenceTerms(t *testing.T) {
	e := NewEscapeBuffer(64)
	var seen []string
	e.AddReportSequenceTerms("\033]", []string{"", "\a", "\033\\"}, func(_ *EscapeBuffer, data []byte) bool {
		seen = append(seen, string(data))
		return false
	})
	e.Write([]byte("a\033]one\033\\b\033]two\ac\033]three\a\033\\d"))
	e.Flush()
	if want := []string{"one", "two", "three"}; strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Errorf("payloads = %q, want %q", seen, want)
	}
	if got, want := string(e.normal), "abc\033\\d"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

func TestAddReportSequenceTermsNone(t *testing.T) {
	e := NewEscapeBuffer(64)
	called := 0
	e.AddReportSequenceTerms("XYZ", []string{""}, func(*EscapeBuffer, []byte) bool {
		called++
		return false
	})
	e.Write([]byte("abXYZcd"))
	e.Flush()
	if called != 1 || string(e.normal) != "abcd" {
		t.Errorf("called %d, buffer %q; want 1, abcd", called, e.normal)
	}
}
