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
	"fmt"
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
	e.AddReportSequenceTerms("\033]", []string{"", "\a", "\033\\"}, "", func(_ *EscapeBuffer, data []byte) bool {
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
	e.AddReportSequenceTerms("XYZ", []string{""}, "", func(*EscapeBuffer, []byte) bool {
		called++
		return false
	})
	e.Write([]byte("abXYZcd"))
	e.Flush()
	if called != 1 || string(e.normal) != "abcd" {
		t.Errorf("called %d, buffer %q; want 1, abcd", called, e.normal)
	}
}

func TestEscapeBreak(t *testing.T) {
	e := NewEscapeBuffer(64)
	e.AddReportSequenceTerms("\033]52;", []string{"\a", "\033\\"}, "", func(*EscapeBuffer, []byte) bool {
		return false
	})
	if e.Break() {
		t.Error("Break with no sequence open returned true")
	}
	e.Write([]byte("a\033]52;c;never ending"))
	e.Write([]byte(" still going\033"))
	if !e.Break() {
		t.Error("Break with a sequence open returned false")
	}
	if e.Break() {
		t.Error("second Break returned true")
	}
	e.Write([]byte("b\033]52;c;?\ac"))
	e.Flush()
	if got, want := string(e.normal), "abc"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

func TestAddReportSequenceTermsEnds(t *testing.T) {
	e := NewEscapeBuffer(64)
	var seen []string
	e.AddReportSequenceTerms("\033]", []string{"\a", "\033\\"}, "\033", func(_ *EscapeBuffer, data []byte) bool {
		seen = append(seen, string(data))
		return false
	})
	// Ends only: no terms at all.
	e.AddReportSequenceTerms("@{", nil, "}", func(_ *EscapeBuffer, data []byte) bool {
		seen = append(seen, string(data))
		return false
	})
	e.Write([]byte("a\033]one\033"))
	e.Write([]byte("\\b\033]two\033"))
	e.Write([]byte("Xc@{three}d"))
	e.Flush()
	if want := []string{"one", "two", "three"}; strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Errorf("payloads = %q, want %q", seen, want)
	}
	if got, want := string(e.normal), "ab\033Xc}d"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

// dropAll is a FilterFunc that records nothing.
func dropAll(*EscapeBuffer, []byte, bool) ([]byte, bool) { return nil, false }

func TestFilterSequenceDropKeepsLittle(t *testing.T) {
	e := NewEscapeBuffer(64)
	e.AddFilterSequence("\033]52;", []string{"\a", "\033\\"}, "\033", 16, dropAll)
	e.Write([]byte("a\033]52;c;"))
	chunk := bytes.Repeat([]byte("QUJD"), 1024)
	for i := 0; i < 100; i++ {
		e.Write(chunk)
		if e.inseq == nil {
			t.Fatal("sequence ended early")
		}
		if n := len(e.inseq.seen); n > 1 {
			t.Fatalf("after %d writes the open sequence holds %d bytes", i+1, n)
		}
	}
	if c := cap(e.inseq.seen); c > 2*len(chunk) {
		t.Errorf("open sequence has capacity %d, want at most %d", c, 2*len(chunk))
	}
	// A terminator split across writes is still found.
	e.Write([]byte("QUJD\033"))
	e.Write([]byte("\\b"))
	e.Flush()
	if got, want := string(e.normal), "ab"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

func TestFilterSequenceEndsOnly(t *testing.T) {
	e := NewEscapeBuffer(64)
	e.AddFilterSequence("@{", nil, "}", 2, dropAll)
	e.Write([]byte("a@{xxx"))
	if e.inseq == nil || len(e.inseq.seen) != 0 {
		t.Fatalf("open sequence = %+v, want one holding nothing", e.inseq)
	}
	e.Write([]byte("yyy}b"))
	e.Flush()
	if got, want := string(e.normal), "a}b"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

func TestFilterSequenceEmpty(t *testing.T) {
	e := NewEscapeBuffer(64)
	e.AddFilterSequence("", []string{"\a"}, "", 0, dropAll)
	if len(e.sequences) != 0 {
		t.Error("empty filter sequence should be ignored")
	}
}

func TestFilterSequenceNoEnd(t *testing.T) {
	e := NewEscapeBuffer(64)
	e.AddFilterSequence("XY", nil, "", 0, func(_ *EscapeBuffer, raw []byte, complete bool) ([]byte, bool) {
		if !complete || string(raw) != "XY" {
			t.Errorf("filter(%q, %v)", raw, complete)
		}
		return []byte("<>"), false
	})
	e.Write([]byte("aXYb"))
	e.Flush()
	if got, want := string(e.normal), "a<>b"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

func TestFilterSequenceComplete(t *testing.T) {
	e := NewEscapeBuffer(64)
	var raws []string
	e.AddFilterSequence("<", []string{">", "]]"}, "", 0, func(_ *EscapeBuffer, raw []byte, complete bool) ([]byte, bool) {
		raws = append(raws, string(raw))
		return []byte(strings.ToUpper(string(raw))), false
	})
	e.Write([]byte("a<x>b<y]"))
	e.Write([]byte("]c"))
	e.Flush()
	if want := []string{"<x>", "<y]]"}; strings.Join(raws, ",") != strings.Join(want, ",") {
		t.Errorf("filter saw %q, want %q", raws, want)
	}
	if got, want := string(e.normal), "a<X>b<Y]]c"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

func TestFilterSequenceStream(t *testing.T) {
	e := NewEscapeBuffer(1024)
	var calls []string
	e.AddFilterSequence("<", []string{">>"}, "", 4, func(_ *EscapeBuffer, raw []byte, complete bool) ([]byte, bool) {
		calls = append(calls, fmt.Sprintf("%q %v", raw, complete))
		return []byte("[" + string(raw[1:]) + "]"), true
	})
	e.Write([]byte("a<123"))
	e.Write([]byte("4567>"))
	e.Write([]byte(">8<9>>b"))
	e.Flush()
	// The first sequence goes past its limit holding "1234567>", of which
	// the last byte may start ">>", so the filter sees "<1234567" and the
	// rest is recorded as it arrives.  The second completes in time.
	if want := []string{`"<1234567" false`, `"<9>>" true`}; strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Errorf("filter calls %q, want %q", calls, want)
	}
	if got, want := string(e.normal), "a[1234567]>>8[9>>]b"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

func TestEndSetEmpty(t *testing.T) {
	if s := newEndSet([]string{""}, ""); s != nil {
		t.Errorf("newEndSet of nothing = %+v, want nil", s)
	}
}

func TestReportSequenceLongTermSplit(t *testing.T) {
	e := NewEscapeBuffer(64)
	var seen []string
	e.AddReportSequence("<", "]]]", func(_ *EscapeBuffer, data []byte) bool {
		seen = append(seen, string(data))
		return false
	})
	e.Write([]byte("a<"))
	e.Write([]byte("x"))
	e.Write([]byte("]]"))
	e.Write([]byte("]b"))
	e.Flush()
	if len(seen) != 1 || seen[0] != "x" {
		t.Errorf("payload = %q, want [x]", seen)
	}
	if got, want := string(e.normal), "ab"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}
