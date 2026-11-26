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
	"time"

	"github.com/pborman/pty/log"
)

// Output goes to every attached client, so when a program queries the
// terminal every attached terminal answers, and every answer arrives as
// input.  Only one answer should reach the program, and an answer, which no
// one typed, should not make its client take the session (and its window
// size).
//
// When the record filters drop a query they tell the Shell what its answer
// will look like (its key, see answerKey).  For answerWindow after that, the
// input from each client is scanned for answers with that key.  Answers are
// passed on from just one client: the primary client, or if there is none,
// the first client to answer.  Answers from the other clients are dropped.
// Input made up only of answers does not make its client take the session.
//
// Input is only scanned while an answer is expected, and a lone ESC at the
// end of a read is never held back, so the escape key is not delayed.

// answerWindow is how long after a query its answers are watched for.
const answerWindow = 5 * time.Second

// ropenAnswer is the key of the answer to ropenQuery, which the terminal
// types as "#ROPEN HOST:PORT/TOKEN NEPOR#".
const (
	ropenAnswer      = "ropen"
	ropenAnswerStart = "#ROPEN "
	ropenAnswerEnd   = " NEPOR#"
)

// answerLimit is the longest answer recognized.  A longer one is passed on
// as if it were typed.
const answerLimit = 4096

// now returns the current time; tests replace it.
var now = time.Now

// An answerWait is an answer the Shell is watching for.
type answerWait struct {
	until time.Time
	from  *Client // the client whose answers are passed on, once chosen
}

// expectAnswer notes that a query whose answer has key has just gone to the
// clients.  It is called by the record filters with s.mu held.
func (s *Shell) expectAnswer(key string) {
	if s.waits == nil {
		s.waits = map[string]*answerWait{}
	}
	// A new query may go to a different primary client, so the
	// choice of client is made again.
	s.waits[key] = &answerWait{until: now().Add(answerWindow)}
}

// FilterInput returns in, input from c, without the answers that are not
// to be passed on.  It reports whether what is returned holds anything other
// than answers, that is, anything the user typed.  F holds the state of c's
// input between calls.
func (s *Shell) FilterInput(c *Client, f *answerFilter, in []byte) ([]byte, bool) {
	defer s.mu.Lock("FilterInput")()
	t := now()
	for key, w := range s.waits {
		if t.After(w.until) {
			delete(s.waits, key)
		}
	}
	if len(s.waits) == 0 && len(f.held) == 0 {
		return in, len(in) > 0
	}
	return f.Filter(in, func(key string) (bool, bool) {
		w := s.waits[key]
		if w == nil {
			return false, false
		}
		if w.from == nil {
			w.from = s.primary
			if w.from == nil {
				w.from = c
			}
		}
		if w.from != c {
			log.Infof("dropping %s answer from client %s", key, c.Name())
		}
		return true, w.from == c
	})
}

// An answerFilter scans one client's input for answers.  A sequence that may
// be an answer but is split across reads is held until the rest arrives.
type answerFilter struct {
	held []byte // the start of a sequence from the end of the last read
	out  []byte // reused output buffer
}

// Filter returns in with each complete answer that decide rejects removed,
// and reports whether what it returns holds anything other than answers.
// Decide is called with the key of each sequence in the input that might be
// an answer, and reports whether one with that key is expected and, if it
// is, whether to keep it.  The returned slice is only valid until the next
// call to Filter.
func (f *answerFilter) Filter(in []byte, decide func(key string) (expected, keep bool)) ([]byte, bool) {
	data := in
	if len(f.held) > 0 {
		data = append(f.held, in...)
		f.held = nil
	}
	out := f.out[:0]
	typed := false
	pass := func(b []byte) {
		if len(b) > 0 {
			out = append(out, b...)
			typed = true
		}
	}
	for i := 0; i < len(data); {
		x := bytes.IndexAny(data[i:], "\033#")
		if x < 0 {
			pass(data[i:])
			break
		}
		pass(data[i : i+x])
		i += x
		rest := data[i:]
		if rest[0] == '#' {
			if expected, _ := decide(ropenAnswer); !expected {
				pass(rest[:1])
				i++
				continue
			}
		}
		n, key, complete := scanAnswer(rest)
		if !complete {
			if n == 1 && rest[0] == '\033' || len(rest) > answerLimit {
				// A lone ESC is the escape key; pass it
				// on now.  Anything this long is not an
				// answer.
				pass(rest)
			} else {
				f.held = append([]byte{}, rest...)
			}
			break
		}
		if key != "" {
			if expected, keep := decide(key); expected {
				if keep {
					out = append(out, rest[:n]...)
				}
				i += n
				continue
			}
		}
		pass(rest[:n])
		i += n
	}
	f.out = out
	return out, typed
}

// scanAnswer looks at the start of b, which starts with ESC or #, for
// something that may be an answer.  It returns its length, its key (""
// if it cannot be an answer), and whether all of it is in b.  If it is not
// complete, n is len(b).
//
// The key of an answer is its kind and the bytes that identify it:
//
//	"CSI " + private + intermediates + final, as in "CSI ?c" for
//	        ESC [ ? 6 2 ; 2 2 c
//	"DCS " + private + intermediates + final, as in "DCS +r" for
//	        ESC P 1 + r 5 4 4 e ESC \
//	"OSC " + Ps, as in "OSC 11" for ESC ] 1 1 ; r g b : ... BEL,
//	        or "OSC l" for the window title, ESC ] l ... ESC \
//	ropenAnswer for #ROPEN ... NEPOR#
func scanAnswer(b []byte) (n int, key string, complete bool) {
	if b[0] == '#' {
		if len(b) < len(ropenAnswerStart) {
			if bytes.HasPrefix([]byte(ropenAnswerStart), b) {
				return len(b), "", false
			}
			return 1, "", true
		}
		if !bytes.HasPrefix(b, []byte(ropenAnswerStart)) {
			return 1, "", true
		}
		x := bytes.Index(b, []byte(ropenAnswerEnd))
		if x < 0 {
			return len(b), "", false
		}
		return x + len(ropenAnswerEnd), ropenAnswer, true
	}
	if len(b) < 2 {
		return len(b), "", false
	}
	switch b[1] {
	case '[':
		for j := 2; j < len(b); j++ {
			switch c := b[j]; {
			case c >= 0x40 && c <= 0x7e:
				return j + 1, csiKey(b[:j+1]), true
			case c == '\030' || c == '\032':
				return j + 1, "", true
			case c == '\033' || c >= 0x80:
				return j, "", true
			}
		}
		return len(b), "", false
	case ']', 'P':
		for j := 2; j < len(b); j++ {
			switch b[j] {
			case '\007':
				if b[1] == ']' {
					return j + 1, stringKey(b[:j]), true
				}
			case '\030', '\032':
				return j + 1, "", true
			case '\033':
				if j+1 == len(b) {
					return len(b), "", false
				}
				if b[j+1] == '\\' {
					return j + 2, stringKey(b[:j]), true
				}
				return j, "", true
			}
		}
		return len(b), "", false
	}
	return 2, "", true
}

// csiKey returns the key of raw, a complete control sequence.
func csiKey(raw []byte) string {
	c, ok := parseCSI(raw)
	if !ok {
		return ""
	}
	key := []byte("CSI ")
	if c.private != 0 {
		key = append(key, c.private)
	}
	key = append(key, c.inter...)
	return string(append(key, c.final))
}

// stringKey returns the key of b, an OSC or DCS without its terminator.
func stringKey(b []byte) string {
	body := b[2:]
	if b[1] == ']' {
		i := 0
		for i < len(body) && body[i] >= '0' && body[i] <= '9' {
			i++
		}
		if i == 0 && len(body) > 0 {
			i = 1 // a title report: l or L
		}
		return "OSC " + string(body[:i])
	}
	// A DCS: private, parameters, intermediates and final, as for CSI.
	key := []byte("DCS ")
	i := 0
	if i < len(body) && body[i] >= '<' && body[i] <= '?' {
		key = append(key, body[i])
		i++
	}
	for i < len(body) && body[i] >= 0x30 && body[i] <= 0x3f {
		i++
	}
	for i < len(body) && body[i] >= 0x20 && body[i] <= 0x2f {
		key = append(key, body[i])
		i++
	}
	if i == len(body) || body[i] < 0x40 || body[i] > 0x7e {
		return ""
	}
	return string(append(key, body[i]))
}
