package ansi

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// An errorList is simply a list of errors.
type errorList []error

func (e errorList) Error() string {
	if len(e) == 0 {
		return ""
	}
	parts := make([]string, len(e))
	for x, err := range e {
		parts[x] = err.Error()
	}
	return strings.Join(parts, "\n")
}

func (e errorList) err() error {
	switch len(e) {
	case 0:
		return nil
	case 1:
		return e[0]
	default:
		return e
	}
}

// Strip returns in with all ANSI escape sequences stripped.  An error is
// also returned if one or more of the stripped escape sequences are invalid.
// Single byte escape sequences are not recognized (UNICODE safe).
func Strip(in []byte) ([]byte, error) {
	d := NewReader(bytes.NewBuffer(in))
	return d.strip()
}

// Strip1 returns in with all ANSI escape sequences stripped.  An error is
// also returned if one or more of the stripped escape sequences are invalid.
// Single byte escape sequences are recognized (UNICODE unsafe).
func Strip1(in []byte) ([]byte, error) {
	d := NewReader(bytes.NewBuffer(in))
	d.AllowOneByteSequences()
	return d.strip()
}

func (bp *Reader) strip() ([]byte, error) {
	var out []string
	var errs errorList
	for {
		s, err := bp.Next()
		if err != nil {
			if err != io.EOF {
				errs = append(errs, err)
			}
			return []byte(strings.Join(out, "")), errs.err()
		}
		if s.Error != nil {
			errs = append(errs, fmt.Errorf("%q: %v", s, s.Error))
		}
		if s.Code == "" {
			out = append(out, s.Text)
		}
	}
}
