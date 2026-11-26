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
	"io"
	"os"
	"sort"
	"strings"

	"github.com/pborman/ansi"
	"github.com/pborman/pty/xterm"
)

func init() {
	if dups := ansi.Import(xterm.Table); len(dups) != 0 {
		for _, d := range dups {
			fmt.Fprintf(os.Stderr, "Duplicate escape sequence: %q\n", d)
		}
		os.Exit(1)
	}
}

var tlog = func(format string, v ...interface{}) {}

// An endSet says how a sequence that does not end with its introducer ends.
type endSet struct {
	multi   [][]byte  // terminators of two or more bytes
	term1   [256]bool // one-byte terminators
	ends    [256]bool // bytes that end the sequence but are not part of it
	first   [256]bool // the first bytes of all of the above
	maxTerm int       // length of the longest terminator
}

// newEndSet returns the endSet for terms and ends (see
// AddReportSequenceTerms), or nil if both are empty.
func newEndSet(terms []string, ends string) *endSet {
	s := &endSet{}
	for _, t := range terms {
		switch len(t) {
		case 0:
			continue
		case 1:
			s.term1[t[0]] = true
		default:
			s.multi = append(s.multi, []byte(t))
		}
		s.first[t[0]] = true
		if len(t) > s.maxTerm {
			s.maxTerm = len(t)
		}
	}
	for i := 0; i < len(ends); i++ {
		s.ends[ends[i]] = true
		s.first[ends[i]] = true
	}
	if s.maxTerm == 0 && len(ends) == 0 {
		return nil
	}
	return s
}

// hold returns the most bytes at the end of what has been seen that may be
// the start of a terminator not yet complete.
func (s *endSet) hold() int {
	if s.maxTerm > 1 {
		return s.maxTerm - 1
	}
	return 0
}

// find looks for the end of a sequence in seen, starting at from.  It
// returns where the sequence's own bytes end and how many bytes after that
// belong to its terminator (0 if it ended before a byte from ends).  It
// returns -1, 0 if the end is not in seen yet.  At any one place a
// multi-byte terminator is preferred to a one-byte terminator, and both to a
// byte from ends.
func (s *endSet) find(seen []byte, from int) (int, int) {
	for i := from; i < len(seen); i++ {
		c := seen[i]
		if !s.first[c] {
			continue
		}
		rest := seen[i:]
		for _, t := range s.multi {
			if bytes.HasPrefix(rest, t) {
				return i, len(t)
			}
		}
		if s.term1[c] {
			return i, 1
		}
		// A multi-byte terminator may yet start here once more
		// bytes arrive.
		for _, t := range s.multi {
			if len(rest) < len(t) && bytes.HasPrefix(t, rest) {
				return -1, 0
			}
		}
		if s.ends[c] {
			return i, 0
		}
	}
	return -1, 0
}

// A FilterFunc decides what to record for a sequence registered with
// AddFilterSequence.  Raw holds the sequence's bytes from its introducer on.
//
// When complete is true raw is the whole sequence, including its terminator
// if it had one, and out is what to record in its place (nil for nothing).
// Keep is ignored.
//
// When complete is false the sequence has grown past its limit and raw is
// only its start.  If keep is true, out is recorded and the rest of the
// sequence is recorded as it arrives, without calling the FilterFunc again.
// If keep is false nothing more of the sequence is kept or recorded.
type FilterFunc func(e *EscapeBuffer, raw []byte, complete bool) (out []byte, keep bool)

type seqCall struct {
	seq      []byte
	end      *endSet // nil if the sequence is complete once seq is seen
	limit    int     // bytes a FilterFunc sequence may hold before deciding
	callback func(*EscapeBuffer, []byte) bool
	filter   FilterFunc

	// The state of an open sequence.
	seen   []byte // bytes seen so far (after seq) and not yet dealt with
	drop   bool   // record nothing more; keep only enough of seen to find the end
	stream bool   // record each byte as it arrives
}

type EscapeBuffer struct {
	normal     []byte
	alt        []byte
	partial    []byte
	inalt      bool
	firstBytes string
	sequences  []seqCall
	inseq      *seqCall // the open sequence (&open), or nil
	open       seqCall
	raw        []byte // scratch space for the raw bytes passed to a FilterFunc

	// Expect, if not nil, is called by a filter that drops a query
	// with the key of the answer the terminal will send.
	Expect func(key string)
}

// expect calls e.Expect, if it is set, with key, if it is not empty.
func (e *EscapeBuffer) expect(key string) {
	if e.Expect != nil && key != "" {
		e.Expect(key)
	}
}

func NewEscapeBuffer(n int) *EscapeBuffer {
	if n <= 0 {
		n = 1024 * 1024
	}
	return &EscapeBuffer{
		normal: make([]byte, 0, n),
		alt:    make([]byte, 0, n),
	}
}

func (e *EscapeBuffer) AddSequence(seq string, f func(*EscapeBuffer) bool) {
	if len(seq) == 0 {
		return
	}
	if strings.IndexByte(e.firstBytes, seq[0]) < 0 {
		e.firstBytes += string(seq[:1])
	}
	e.sequences = append(e.sequences, seqCall{
		seq: []byte(seq),
		callback: func(e *EscapeBuffer, _ []byte) bool {
			return f(e)
		},
	})
}

// AddReportSequence arranges for f to be called with the bytes between seq
// and the next term.  The bytes from seq through term are recorded only if f
// returns true, and then without seq and term.  An empty term makes it the
// same as AddSequence.
func (e *EscapeBuffer) AddReportSequence(seq, term string, f func(*EscapeBuffer, []byte) bool) {
	e.AddReportSequenceTerms(seq, []string{term}, "", f)
}

// AddReportSequenceTerms is AddReportSequence for a sequence that may end
// with any one of terms, or just before any byte in ends.  A byte from ends
// is not part of the sequence: it is processed again as ordinary output (for
// an OSC, ESC is such a byte, as it starts the next escape sequence).  Where
// a term and a byte from ends start at the same place, the term wins, so a
// term may begin with a byte from ends (as ST, ESC \, does).  Empty strings
// in terms are ignored.
//
// A terminator is found even when it is split across calls to Write.
func (e *EscapeBuffer) AddReportSequenceTerms(seq string, terms []string, ends string, f func(*EscapeBuffer, []byte) bool) {
	if len(seq) == 0 {
		return
	}
	end := newEndSet(terms, ends)
	if end == nil {
		e.AddSequence(seq, func(*EscapeBuffer) bool {
			return f(e, nil)
		})
		return
	}
	e.addSeqCall(seqCall{
		seq:      []byte(seq),
		end:      end,
		callback: f,
	})
}

// AddFilterSequence arranges for f to decide what is recorded for each
// sequence that starts with seq and ends as described for
// AddReportSequenceTerms.  F is first called once the sequence is complete
// or, if limit is positive, once more than limit bytes past seq have arrived
// without it completing.  With no terms or ends, the sequence is just seq.
func (e *EscapeBuffer) AddFilterSequence(seq string, terms []string, ends string, limit int, f FilterFunc) {
	if len(seq) == 0 {
		return
	}
	e.addSeqCall(seqCall{
		seq:    []byte(seq),
		end:    newEndSet(terms, ends),
		limit:  limit,
		filter: f,
	})
}

func (e *EscapeBuffer) addSeqCall(s seqCall) {
	if strings.IndexByte(e.firstBytes, s.seq[0]) < 0 {
		e.firstBytes += string(s.seq[:1])
	}
	e.sequences = append(e.sequences, s)
}

// filterRaw calls s.filter with s.seq followed by body.
func (e *EscapeBuffer) filterRaw(s *seqCall, body []byte, complete bool) ([]byte, bool) {
	e.raw = append(append(e.raw[:0], s.seq...), body...)
	return s.filter(e, e.raw, complete)
}

// trim discards all but the last few bytes of s.seen, the most that can be
// the start of a terminator not yet complete.
func (s *seqCall) trim() {
	if keep := s.end.hold(); len(s.seen) > keep {
		s.seen = append(s.seen[:0], s.seen[len(s.seen)-keep:]...)
	}
}

func appendto(old, new []byte) []byte {
	nl := len(new)
	ol := len(old)
	oc := cap(old)
	switch {
	case nl == 0:
	case nl >= oc:
		old = old[:oc]
		copy(old, new[nl-oc:])
	case nl+ol < oc:
		old = append(old, new...)
	default:
		extra := 1024
		if extra > oc/8 {
			extra = oc / 8
			if extra == 0 {
				extra = 1
			}
		}
		extra = nl + ol - oc + extra
		old = old[:copy(old[:oc], old[extra:])]
		old = append(old, new...)
	}
	return old
}

// Break abandons the report sequence in progress, if any, discarding what
// it has seen, and reports whether there was one.  It is how a sequence that
// never ends (a program that died or misbehaved part way through an OSC) is
// stopped from swallowing all later output from the record.
func (e *EscapeBuffer) Break() bool {
	if e.inseq == nil {
		return false
	}
	e.inseq = nil
	return true
}

func (e *EscapeBuffer) Flush() {
	if e.inalt {
		e.alt = appendto(e.alt, e.partial)
	} else {
		e.normal = appendto(e.normal, e.partial)
	}
}

func (e *EscapeBuffer) add(buf []byte) {
	if e.inalt {
		e.alt = appendto(e.alt, buf)
	} else {
		e.normal = appendto(e.normal, buf)
	}
}

func (e *EscapeBuffer) Write(buf []byte) (int, error) {
	n := len(buf)

	if e.firstBytes == "" {
		e.add(buf)
		return n, nil
	}
	// If e.partial has length then its cap is how many bytes we
	// need to match the longest partial sequence match.
	// Fill up the partial buffer with as many bytes as we can,
	// recurse, and try again.  If buf cannot fully fill
	// our partial buffer and we still just have a possible prefix
	// then e.partial will end up with the partial buffer and
	// the length of buf will be zero.
	for len(buf) > 0 && len(e.partial) > 0 {
		pl := len(e.partial)
		i := copy(e.partial[pl:cap(e.partial)], buf)
		buf = buf[i:]
		ep := e.partial[:pl+i]
		tlog("partial[%d:%d] write: %q, leaving %q", len(e.partial), cap(e.partial), ep, buf)
		e.partial = nil
		e.Write(ep)
	}

	// If e.partial is still not empty then we can't know if
	// we have hit a sequence or not so we just return.  We
	// know buf has nothing in it if e.partial has something.
	if len(e.partial) > 0 {
		tlog("return on partial: %q", e.partial)
		return n, nil
	}
Loop:
	for {
		// If we are in the middle of an escape sequence, wait
		// for the ending bytes.
		if e.inseq != nil {
			// A terminator may be split across writes, so
			// search what has been seen so far, starting far
			// enough back to catch one that began in an
			// earlier write.  A terminator that ended in an
			// earlier write would already have been found.
			// Most sequences start and end in one write; for
			// them buf is searched where it is, without
			// copying it.
			seq := e.inseq
			hold := seq.end.hold()
			data, from := buf, 0
			if len(seq.seen) > 0 {
				from = len(seq.seen) - hold
				if from < 0 {
					from = 0
				}
				seq.seen = append(seq.seen, buf...)
				data = seq.seen
			}
			x, tl := seq.end.find(data, from)
			if x < 0 {
				if len(seq.seen) == 0 {
					seq.seen = append(seq.seen, buf...)
				}
				switch {
				case seq.drop:
				case seq.stream:
					e.add(seq.seen[:len(seq.seen)-hold])
				case seq.filter != nil && seq.limit > 0 && len(seq.seen) > seq.limit:
					out, keep := e.filterRaw(seq, seq.seen[:len(seq.seen)-hold], false)
					if keep {
						e.add(out)
						seq.stream = true
					} else {
						seq.drop = true
					}
				default:
					return n, nil
				}
				seq.trim()
				return n, nil
			}
			e.inseq = nil
			switch {
			case seq.drop:
			case seq.stream:
				e.add(data[:x+tl])
			case seq.filter != nil:
				out, _ := e.filterRaw(seq, data[:x+tl], true)
				e.add(out)
			case seq.callback(e, data[:x]):
				e.add(data[:x])
			}
			buf = data[x+tl:]
		}

		x := bytes.IndexAny(buf, e.firstBytes)
		if x < 0 {
			e.add(buf)
			return n, nil
		}
		e.add(buf[:x])
		buf = buf[x:]
		maxPartial := 0
		e.partial = nil
		for _, s := range e.sequences {
			if len(buf) >= len(s.seq) {
				if bytes.Equal(buf[:len(s.seq)], s.seq) {
					if s.end != nil {
						// Only one sequence is open at a
						// time, so e.open holds it.
						e.open = s
						e.inseq = &e.open
					} else if s.filter != nil {
						out, _ := s.filter(e, s.seq, true)
						e.add(out)
					} else if s.callback(e, nil) {
						e.add(s.seq)
					}
					buf = buf[len(s.seq):]
					continue Loop
				}
				continue
			}
			// Our sequence is longer than the buffer.
			// If it is a prefix then buf might be a
			// partial match.
			if bytes.Equal(s.seq[:len(buf)], buf) {
				if len(s.seq) > maxPartial {
					maxPartial = len(s.seq)
				}
			}
		}
		// If we got a partial match then we will have to save
		// this buffer for the next call to write.
		if maxPartial > 0 {
			e.partial = make([]byte, len(buf), maxPartial)
			copy(e.partial, buf)
			return n, nil
		}
		e.add(buf[:1])
		buf = buf[1:]
	}
}

func (e *EscapeBuffer) sendEscapes(w io.Writer, alt bool) {
	if alt {
		writeEscapes(w, e.alt)
	} else {
		writeEscapes(w, e.normal)
	}
}

// writeEscapes writes to w the list of the distinct escape sequences in buf.
func writeEscapes(w io.Writer, buf []byte) {
	seen := map[string]bool{}
	// UTF8 keeps multibyte runes as text (bytes 0x80-0x9f are not treated
	// as C1 controls); only real escape sequences (non-empty Type) count.
	for _, s := range (ansi.Decoder{UTF8: true}).DecodeAll(buf) {
		if s.Type != "" {
			seen[string(s.Code)] = true
		}
	}
	codes := make([]string, 0, len(seen))
	for code := range seen {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	maxlen := 0
	fmt.Fprintf(w, "-----\r\n")
	for _, code := range codes {
		if n := len(fmt.Sprintf("%q", code)); n > maxlen {
			maxlen = n
		}
	}
	for _, code := range codes {
		if code == "" {
			continue
		}
		seq := ansi.Table[ansi.Name(code)]
		if seq != nil {
			fmt.Fprintf(w, "Code: %-*q %s\r\n", maxlen, code, seq.Name)
		} else {
			fmt.Fprintf(w, "Code: %q\r\n", code)
		}
	}
	fmt.Fprintf(w, "-----\r\n")
}
