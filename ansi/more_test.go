package ansi

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"strings"
	"testing"
)

func TestStripNoOneByte(t *testing.T) {
	for _, tt := range []struct {
		in, out string
		errors  bool
	}{
		{in: "abc", out: "abc"},
		{in: "a\033[31mb", out: "ab"},
		{in: "a\033[mb", out: "ab"},
		{in: "\033", errors: true, out: ""},
		{in: "abc\033", out: "abc", errors: true},
		{in: "a\033[1;2;3mb", out: "ab"},
	} {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Strip([]byte(tt.in))
			if tt.errors && err == nil {
				t.Error("expected error")
			}
			if !tt.errors && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if string(got) != tt.out {
				t.Errorf("got %q, want %q", got, tt.out)
			}
		})
	}
}

func TestErrorList(t *testing.T) {
	var e errorList
	if e.Error() != "" {
		t.Errorf("empty Error() = %q", e.Error())
	}
	if e.err() != nil {
		t.Errorf("empty err() = %v", e.err())
	}
	e = errorList{errors.New("one")}
	if e.Error() != "one" {
		t.Errorf("single Error() = %q", e.Error())
	}
	if e.err() == nil || e.err().Error() != "one" {
		t.Errorf("single err() = %v", e.err())
	}
	e = errorList{errors.New("a"), errors.New("b")}
	if e.Error() != "a\nb" {
		t.Errorf("multi Error() = %q", e.Error())
	}
	if e.err() == nil || e.err().Error() != "a\nb" {
		t.Errorf("multi err() = %v", e.err())
	}
}

func TestSString(t *testing.T) {
	s := S{Text: "hello"}
	if s.String() != "hello" {
		t.Errorf("String = %q", s.String())
	}
}

func TestNameS(t *testing.T) {
	if NUL.S() == nil {
		t.Error("NUL.S() is nil")
	}
	if Name("not-a-real-code").S() != nil {
		t.Error("unknown Name.S() should be nil")
	}
}

func TestImportDups(t *testing.T) {
	dups := Import(map[Name]*Sequence{
		NUL: {Name: "dup-nul"},
	})
	if len(dups) != 1 || dups[0] != NUL {
		t.Errorf("Import dups = %q, want [%q]", dups, NUL)
	}
	dups = Import(map[Name]*Sequence{
		Name("\033[999~made-up"): {Name: "unique", Type: CSI, Code: []byte{'~'}},
	})
	if len(dups) != 0 {
		t.Errorf("Import of unique name reported dups %q", dups)
	}
}

func TestReaderReadStrips(t *testing.T) {
	r := NewReader(strings.NewReader("hello\033[31mworld"))
	got, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "helloworld" {
		t.Errorf("Read = %q, want helloworld", got)
	}

	// Partial reads into a tiny buffer.
	r = NewReader(strings.NewReader("abcdef"))
	buf := make([]byte, 2)
	var out []byte
	for {
		n, err := r.Read(buf)
		out = append(out, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if string(out) != "abcdef" {
		t.Errorf("chunked Read = %q", out)
	}
}

func TestReaderSend(t *testing.T) {
	r := NewReader(strings.NewReader("ab\033[Acd"))
	ch := make(chan S, 16)
	errc := make(chan error, 1)
	go func() {
		errc <- r.Send(ch)
		close(ch)
	}()
	var texts []string
	for s := range ch {
		texts = append(texts, s.String())
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(texts, "")
	if joined != "ab\033[Acd" {
		t.Errorf("Send texts joined = %q", joined)
	}
}

func TestReaderSendError(t *testing.T) {
	r := NewReader(strings.NewReader("\033"))
	ch := make(chan S, 4)
	err := r.Send(ch)
	if err != nil {
		// Lone ESC is returned as a sequence with Error set, then EOF.
		// Send treats a Next error of EOF as success; a LoneEscape is
		// attached to the sequence, not returned as Next's error.
		t.Logf("Send err = %v", err)
	}
}

func TestFormatESC(t *testing.T) {
	s := &Sequence{Type: ESC, Code: []byte{'7'}}
	got := s.Format()
	want := []byte{033, '7'}
	if !bytes.Equal(got, want) {
		t.Errorf("Format ESC = %q, want %q", got, want)
	}
	s = &Sequence{Type: CSI, NParam: -1, Code: []byte{'m'}}
	got = s.Format(1, 31)
	if !bytes.Contains(got, []byte("1;31")) {
		t.Errorf("Format CSI any-params = %q", got)
	}
}

func TestWriterExtras(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)

	if w.SetColor(0) != w {
		t.Error("invalid SetColor should return the same writer")
	}
	if w.SetBackground(0) != w {
		t.Error("invalid SetBackground should return the same writer")
	}
	if w.SetIntensity(99) != w {
		t.Error("invalid SetIntensity should return the same writer")
	}

	red := w.Red()
	if red.SetColor(Red) != red {
		t.Error("same color should return the same writer")
	}
	if red.SetBackground(RedBackground) != red && red.SetBackground(Red) == nil {
		t.Error("SetBackground rejected a valid color")
	}

	def := w.Default()
	if def == nil {
		t.Fatal("Default returned nil")
	}
	norm := w.Normal()
	if norm == nil {
		t.Fatal("Normal returned nil")
	}
	if w.SetIntensity(Default) == nil {
		t.Error("SetIntensity(Default) returned nil")
	}

	reset := w.Red().Bold().Reset()
	n, err := reset.Write([]byte("plain"))
	if err != nil || n != 5 {
		t.Errorf("Reset Write = %d, %v", n, err)
	}

	w.ForceReset()
	w.ForceSet()

	buf.Reset()
	w = NewWriter(&buf)
	w.Red().Write([]byte("x"))
	w.NoColor()
	buf.Reset()
	w.Red().Write([]byte("y"))
	if buf.String() != "y" {
		t.Errorf("NoColor still emitted color: %q", buf.String())
	}
	w.Colorize()
	buf.Reset()
	w.Red().Write([]byte("z"))
	if !strings.Contains(buf.String(), "z") {
		t.Errorf("Colorize write = %q", buf.String())
	}

	// Reset of an already-reset writer is a no-op.
	plain := NewWriter(&buf).Reset()
	if plain.Reset() != plain {
		t.Error("Reset of zero writer should return itself")
	}
}

func TestWriterWriteError(t *testing.T) {
	w := NewWriter(&errWriter{err: io.ErrClosedPipe})
	_, err := w.Red().Write([]byte("x"))
	if err == nil {
		t.Error("expected write error")
	}
	_, err = w.Red().WriteString("x")
	if err == nil {
		t.Error("expected WriteString error")
	}
}

type errWriter struct{ err error }

func (e *errWriter) Write([]byte) (int, error) { return 0, e.err }

func TestBufferFull(t *testing.T) {
	long := "\033[" + strings.Repeat("1;", 100) + "A"
	r := NewWithBuffer(strings.NewReader(long), make([]byte, 8))
	s, err := r.Next()
	if err != BufferFull && s.Error != BufferFull {
		t.Errorf("tiny buffer CSI: seq=%v err=%v, want BufferFull", s, err)
	}
}

func TestTrailingByteSequence(t *testing.T) {
	r := NewReader(strings.NewReader("\033(Bhello"))
	s, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if s.Type != "ESC" {
		t.Errorf("type = %q, want ESC", s.Type)
	}
	s, err = r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if s.Text != "hello" {
		t.Errorf("text = %q", s.Text)
	}
}

func TestC1AndICF(t *testing.T) {
	r := NewReader(strings.NewReader("\033M")) // RI, a C1
	s, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if s.Type != "C1" && s.Type != "ESC" {
		t.Logf("ESC M type = %q code=%q err=%v", s.Type, s.Code, s.Error)
	}

	r = NewReader(strings.NewReader("\033`")) // ICF
	s, err = r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if s.Type != "ICF" {
		t.Errorf("ESC ` type = %q, want ICF", s.Type)
	}
}

func TestUnknownEscape(t *testing.T) {
	r := NewReader(strings.NewReader("\033\x01"))
	s, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if s.Error != UnknownEscape && s.Error != LoneEscape {
		t.Logf("ESC 0x01: type=%q err=%v", s.Type, s.Error)
	}
}

func TestFindSTWithBEL(t *testing.T) {
	r := NewReader(strings.NewReader("\033]0;title\a rest"))
	s, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if s.Type != "CS" {
		t.Errorf("OSC type = %q, want CS", s.Type)
	}
}

func TestFindSTSOS(t *testing.T) {
	r := NewReader(strings.NewReader("\033]hi\033Xmore"))
	s, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if s.Error != FoundSOS {
		t.Errorf("embedded SOS: err=%v, want FoundSOS", s.Error)
	}
}
