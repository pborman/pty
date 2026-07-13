package ansi

import (
	"strings"
	"testing"
)

// TestFindSTTruncatedControlString tests that a single C1 control-string
// introducer (DCS 0x90, OSC 0x9d, PM 0x9e, APC 0x9f, SOS 0x98) with
// AllowOneByteSequences enabled enters findST with an escape-sequence length
// (esl) of 1 and a one-byte buffer.  With the old hard-coded txt[2:] slice this
// produced "slice bounds out of range [2:1]".
func TestFindSTTruncatedControlString(t *testing.T) {
	// Every byte whose flipCode maps to a control-string introducer
	// (see lookup[]: DCS 'P', OSC ']', PM '^', APC '_', SOS 'X').
	oneByte := []byte{0x90, 0x9d, 0x9e, 0x9f, 0x98}

	for _, b := range oneByte {
		in := []byte{b}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Strip1(%#x) panicked: %v", b, r)
				}
			}()
			out, err := Strip1(in)
			// The introducer is a recognized escape, so it is
			// stripped from the output entirely.
			if len(out) != 0 {
				t.Errorf("Strip1(%#x) = %q, want empty", b, out)
			}
			// It is a truncated control string: missing ST.
			if err == nil || !strings.Contains(err.Error(), NoST.Error()) {
				t.Errorf("Strip1(%#x) err = %v, want %v", b, err, NoST)
			}
		}()
	}
}

// TestFindSTTruncatedTwoByte covers the two-byte (esl==2) introducer,
// e.g. ESC P (DCS), to confirm the esl-based slice is correct there too.
func TestFindSTTruncatedTwoByte(t *testing.T) {
	// ESC P with no terminator.
	in := []byte{escape, 'P'}
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Strip1(ESC P) panicked: %v", r)
			}
		}()
		out, err := Strip1(in)
		if len(out) != 0 {
			t.Errorf("Strip1(ESC P) = %q, want empty", out)
		}
		if err == nil || !strings.Contains(err.Error(), NoST.Error()) {
			t.Errorf("Strip1(ESC P) err = %v, want %v", err, NoST)
		}
	}()
}
