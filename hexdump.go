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

// The hexdump tap records the bytes flowing through the client when
// --hexdump=FILE is given.  Three streams are recorded:
//
//	IN   bytes read from the terminal, exactly as the terminal sent them
//	SND  bytes handed to the server, after escape stripping and --grok mapping
//	OUT  bytes about to be written to the terminal, as received from the server
//
// IN and SND are recorded separately on purpose: the difference between them is
// everything this client does to the user's keystrokes, which is the first
// thing to rule out when a full screen application misbehaves under pty but
// works when run directly.
//
// All three streams share one file and one lock, so the order of records in the
// file is the order the bytes crossed the client.  That ordering is the point.
// An application that appears to hang is usually blocked reading a reply to a
// query it wrote; the file shows the query leaving in OUT and whether anything
// ever came back in IN.
//
// Writes are unbuffered so that a session killed with a signal still leaves a
// complete file behind.

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pborman/ansi"
	"github.com/pborman/pty/mutex"
)

type hexdumper struct {
	mu  *mutex.Mutex
	w   *os.File
	off map[string]int64
}

var hexdumps = hexdumper{
	mu:  mutex.New("hexdumper"),
	off: map[string]int64{},
}

// Open starts dumping to path, truncating any existing file.
func (h *hexdumper) Open(path string) error {
	w, err := os.Create(path)
	if err != nil {
		return err
	}
	defer h.mu.Lock("Open")()
	h.w = w
	fmt.Fprintf(w, "# pty hexdump started %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(w, "# IN  terminal -> client (raw keystrokes)\n")
	fmt.Fprintf(w, "# SND client -> server (after escape stripping and --grok mapping)\n")
	fmt.Fprintf(w, "# OUT server -> terminal\n")
	return nil
}

// Dump records buf as having crossed the client in the direction dir, which
// should be one of IN, SND or OUT.  Dump is a no-op when --hexdump was not
// given.
func (h *hexdumper) Dump(dir string, buf []byte) {
	if len(buf) == 0 {
		return
	}
	defer h.mu.Lock("Dump")()
	if h.w == nil {
		return
	}
	off := h.off[dir]
	h.off[dir] = off + int64(len(buf))

	fmt.Fprintf(h.w, "\n%s %s %d bytes @%#x\n",
		time.Now().Format("15:04:05.000000"), dir, len(buf), off)
	h.hex(buf)
	if seqs := decodeSeqs(buf); seqs != "" {
		fmt.Fprintf(h.w, "ansi %s\n", seqs)
	}
}

// hex writes buf in the traditional 16 bytes per line hex and ASCII layout.
func (h *hexdumper) hex(buf []byte) {
	for i := 0; i < len(buf); i += 16 {
		line := buf[i:min(i+16, len(buf))]
		var hex, txt strings.Builder
		for j := 0; j < 16; j++ {
			if j == 8 {
				hex.WriteByte(' ')
			}
			if j < len(line) {
				fmt.Fprintf(&hex, "%02x ", line[j])
				if line[j] >= 0x20 && line[j] < 0x7f {
					txt.WriteByte(line[j])
				} else {
					txt.WriteByte('.')
				}
			} else {
				hex.WriteString("   ")
			}
		}
		fmt.Fprintf(h.w, "%04x  %s |%s|\n", i, hex.String(), txt.String())
	}
}

// decodeSeqs renders the escape sequences and control characters in buf by
// name.  It returns "" for buffers that are nothing but printable text, since
// naming those adds no information the ASCII column does not already carry.
//
// A named sequence is only trustworthy when it decoded cleanly.  init imports
// the xterm table into the ansi table, which makes some private sequences
// resolve to an unrelated standard name: the kitty keyboard query "ESC [ ? u"
// matches the entry for SCORC, differing only in its parameter.  Anything that
// decoded with an error is therefore shown as raw bytes rather than under a
// name that would send the reader in the wrong direction.
func decodeSeqs(buf []byte) string {
	var parts []string
	interesting := false
	for _, s := range ansi.DecodeAll(buf) {
		if s == nil {
			continue
		}
		if s.Type == "" {
			// A run of text between sequences.  Control characters
			// arrive here too, and naming them is the whole point
			// when a full screen application is mishandling erase
			// or return.
			text, ctl := textRun([]byte(s.Code))
			parts = append(parts, text...)
			interesting = interesting || ctl
			continue
		}
		interesting = true
		part := fmt.Sprintf("%q", string(s.Code))
		if seq := ansi.Table[s.Code]; seq != nil && seq.Name != "" && s.Error == nil {
			part = seq.Name
		}
		if len(s.Params) > 0 {
			part += "(" + strings.Join(s.Params, ",") + ")"
		}
		if s.Error != nil {
			part += fmt.Sprintf("[%v]", s.Error)
		}
		parts = append(parts, part)
	}
	if !interesting {
		return ""
	}
	return strings.Join(parts, " | ")
}

// textRun splits a run of non-escape bytes into quoted printable stretches and
// individually named control characters.  It reports whether any control
// character was present.
func textRun(buf []byte) (parts []string, ctl bool) {
	var text []byte
	flush := func() {
		if len(text) > 0 {
			parts = append(parts, fmt.Sprintf("%q", string(text)))
			text = text[:0]
		}
	}
	for _, c := range buf {
		if c >= 0x20 && c < 0x7f {
			text = append(text, c)
			continue
		}
		flush()
		ctl = true
		parts = append(parts, ctlName(c))
	}
	flush()
	return parts, ctl
}

// ctlName names a single control character.  DEL is not in the ansi table but
// is exactly the byte most terminals send for the erase key, so it is named
// here rather than printed as a bare number.
func ctlName(c byte) string {
	if c == 0x7f {
		return "DEL"
	}
	if seq := ansi.Table[ansi.Name([]byte{c})]; seq != nil && seq.Name != "" {
		return seq.Name
	}
	return fmt.Sprintf("\\x%02x", c)
}
