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

	"github.com/pborman/pty/ansi"
	"github.com/pborman/pty/ansi/xterm"
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

type seqCall struct {
	seq      []byte
	term     []byte // bytes that terminate the sequence
	seen     []byte // bytes we have seen so far
	callback func(*EscapeBuffer, []byte) bool
}

type EscapeBuffer struct {
	normal     []byte
	alt        []byte
	partial    []byte
	inalt      bool
	firstBytes string
	sequences  []seqCall
	inseq      *seqCall
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

func (e *EscapeBuffer) AddReportSequence(seq, term string, f func(*EscapeBuffer, []byte) bool) {
	if len(seq) == 0 {
		return
	}
	if len(term) == 0 {
		e.AddSequence(seq, func(*EscapeBuffer) bool {
			return f(e, nil)
		})
		return
	}

	if strings.IndexByte(e.firstBytes, seq[0]) < 0 {
		e.firstBytes += string(seq[:1])
	}
	e.sequences = append(e.sequences, seqCall{
		seq:      []byte(seq),
		term:     []byte(term),
		callback: f,
	})
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

func (e *EscapeBuffer) Flush() {
	if e.inalt {
		e.alt = appendto(e.alt, e.partial)
	} else {
		e.normal = appendto(e.normal, e.partial)
	}
}

func (e *EscapeBuffer) Write(buf []byte) (int, error) {
	n := len(buf)

	add := func(buf []byte) {
		e.normal = appendto(e.normal, buf)
	}
	if e.inalt {
		add = func(buf []byte) {
			e.alt = appendto(e.alt, buf)
		}
	}

	if e.firstBytes == "" {
		add(buf)
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
			// there is a bug here if the terminating
			// sequence is more than one byte.
			x := bytes.Index(buf, e.inseq.term)
			if x < 0 {
				e.inseq.seen = append(e.inseq.seen, buf...)
				return n, nil
			}
			e.inseq.seen = append(e.inseq.seen, buf[:x]...)
			x += len(e.inseq.term)
			if e.inseq.callback(e, e.inseq.seen) {
				add(e.inseq.seen)
			}
			e.inseq = nil
		}

		x := bytes.IndexAny(buf, e.firstBytes)
		if x < 0 {
			add(buf)
			return n, nil
		}
		add(buf[:x])
		buf = buf[x:]
		maxPartial := 0
		e.partial = nil
		for _, s := range e.sequences {
			if len(buf) >= len(s.seq) {
				if bytes.Equal(buf[:len(s.seq)], s.seq) {
					if len(s.term) > 0 {
						seq := s
						e.inseq = &seq
					} else if s.callback(e, nil) {
						add(s.seq)
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
		add(buf[:1])
		buf = buf[1:]
	}
}

func (e *EscapeBuffer) sendEscapes(w io.Writer, alt bool) {
	var buf []byte
	if alt {
		buf = e.alt
	} else {
		buf = e.normal
	}
	r := ansi.NewReader(bytes.NewBuffer(buf))
	ch := make(chan ansi.S)
	go func() {
		r.Send(ch)
		close(ch)
	}()
	seen := map[string]bool{}
	for s := range ch {
		seen[string(s.Code)] = true
	}
	codes := make([]string, 0, len(seen))
	for code := range seen {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	fmt.Fprintf(w, "-----\r\n")
	for _, code := range codes {
		if code == "" {
			continue
		}
		seq := ansi.Table[ansi.Name(code)]
		if seq != nil {
			fmt.Fprintf(w, "Code: %q %s\r\n", code, seq.Name)
		} else {
			fmt.Fprintf(w, "Code: %q\r\n", code)
		}
	}
	fmt.Fprintf(w, "-----\r\n")
}
