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

import "strings"

// An escapeKey finds the client's escape commands in what the user types:
// the escape character (^P by default) followed by . (disconnect) or :
// (run a command).  The escape character twice sends one to the shell.  The
// escape character followed by anything else sends both on unchanged.
//
// An escape character at the end of one read is held until the next read
// shows what follows it.
type escapeKey struct {
	key  byte // the escape character; 0 means there is none
	held bool // the last read ended with key
	out  []byte
}

// Scan scans in, the next bytes typed.  It returns the bytes to send to the
// shell, and if it finds a command, the command ('.' or ':') and the bytes
// typed after it, which Scan has not looked at.  Out is only valid until the
// next call to Scan; rest is a slice of in.
func (e *escapeKey) Scan(in []byte) (out []byte, cmd byte, rest []byte) {
	if e.key == 0 {
		return in, 0, nil
	}
	out = e.out[:0]
	defer func() { e.out = out[:0] }()
	for i, c := range in {
		if !e.held {
			if c == e.key {
				e.held = true
			} else {
				out = append(out, c)
			}
			continue
		}
		e.held = false
		switch c {
		case '.', ':':
			return out, c, in[i+1:]
		case e.key:
			out = append(out, e.key)
		default:
			out = append(out, e.key, c)
		}
	}
	return out, 0, nil
}

// lineFrom returns the line that begins with typed, the bytes typed after
// the : command in the same read, followed by what read reads, which is a
// line from the terminal without its newline.  Typed is echoed, since the
// terminal was not echoing when it was read.  Anything in typed after the
// line's end is returned as rest.  As with readline, the line is returned
// without leading or trailing space.
func lineFrom(typed []byte, echo func([]byte), read func() (string, error)) (line string, rest []byte, err error) {
	for i, c := range typed {
		if c == '\r' || c == '\n' {
			echo(typed[:i])
			echo([]byte("\r\n"))
			return strings.TrimSpace(string(typed[:i])), typed[i+1:], nil
		}
	}
	echo(typed)
	s, err := read()
	return strings.TrimSpace(string(typed) + s), nil, err
}
