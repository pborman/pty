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
	"encoding/base64"

	"github.com/pborman/pty/log"
)

// The clear sequence lets the terminal tell pty that it has cleared its
// screen and scrollback (iPadSSH sends it on Cmd-K).  The terminal sends,
// as input,
//
//	ESC ] 7720 ; clear ; BASE64 ST
//
// where ST is ESC \ or BEL and BASE64 (standard encoding, possibly empty)
// holds what the terminal's screen now shows: ordinary terminal output
// ending with a cursor position.  The pty client removes the sequence from
// its input and sends the decoded bytes to the server in a clearMessage.
// If the normal screen is active the server replaces everything recorded
// for it with clearPrefix followed by the decoded bytes, and writes
// ESC [ 3 J plus the new record back to that client only.  If the
// alternate screen is active, or the base64 is bad, nothing changes and
// nothing is written back.
const (
	clearIntro  = "\033]7720;clear;"
	clearPrefix = nsbrc + home + edall
)

// A clearFilter removes clear sequences from the client's input.  The
// sequence may be split across reads, so the filter keeps state between
// calls to Filter.
type clearFilter struct {
	matched int    // bytes of clearIntro matched so far
	inBody  bool   // clearIntro seen; collecting base64
	sawESC  bool   // in the body, an ESC that may start ESC \
	body    []byte // base64 collected so far
	out     []byte // reused output buffer
}

func isBase64(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' ||
		c == '+' || c == '/' || c == '='
}

// Filter returns in with every clear sequence removed, calling clear with
// the decoded bytes of each complete, valid one.  The returned slice is only
// valid until the next call to Filter.
//
// A partial match of clearIntro at the end of in is held back until the next
// call, with one exception: a lone ESC is never held, so that the escape key
// is not delayed.  A body byte that is neither base64 nor the terminator
// discards the sequence; that byte is then treated as ordinary input.
func (f *clearFilter) Filter(in []byte, clear func([]byte)) []byte {
	out := f.out[:0]
	for i := 0; i < len(in); i++ {
		c := in[i]
		if f.inBody {
			switch {
			case f.sawESC && c == '\\', !f.sawESC && c == '\a':
				f.finish(clear)
				continue
			case !f.sawESC && c == '\033':
				f.sawESC = true
				continue
			case !f.sawESC && isBase64(c):
				f.body = append(f.body, c)
				continue
			}
			// Not part of a clear sequence: drop what was
			// collected and treat c as ordinary input.  A
			// pending ESC is input too; as clearIntro[0] it
			// may even start a new clear sequence.
			wasESC := f.sawESC
			f.reset()
			if wasESC {
				f.matched = 1
			}
		}
		if c == clearIntro[f.matched] {
			f.matched++
			if f.matched == len(clearIntro) {
				f.matched = 0
				f.inBody = true
			}
			continue
		}
		if f.matched > 0 {
			// Release the false start and look at c afresh.
			out = append(out, clearIntro[:f.matched]...)
			f.matched = 0
			i--
			continue
		}
		out = append(out, c)
	}
	if f.matched == 1 {
		// A lone ESC at the end of a read is almost certainly the
		// escape key; pass it on now.
		out = append(out, clearIntro[0])
		f.matched = 0
	}
	f.out = out
	return out
}

func (f *clearFilter) finish(clear func([]byte)) {
	data, err := base64.StdEncoding.DecodeString(string(f.body))
	f.reset()
	if err != nil {
		log.Warnf("clear sequence: bad base64: %v", err)
		return
	}
	clear(data)
}

func (f *clearFilter) reset() {
	f.inBody = false
	f.sawESC = false
	f.body = f.body[:0]
}

// ClearScreen handles a clearMessage from client c carrying screen, the
// decoded contents of a clear sequence.
func (s *Shell) ClearScreen(c *Client, screen []byte) {
	defer s.mu.Lock("ClearScreen")()
	if s.eb.inalt {
		log.Infof("clear sequence ignored: alternate screen is active")
		return
	}
	record := make([]byte, 0, len(clearPrefix)+len(screen))
	record = append(record, clearPrefix...)
	record = append(record, screen...)
	s.eb.normal = appendto(s.eb.normal[:0], record)
	c.Output(append([]byte(edsaved), s.eb.normal...))
}
