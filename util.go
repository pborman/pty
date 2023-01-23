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
	"os"
	"strconv"

	"github.com/pborman/pty/log"
	"github.com/pborman/pty/mutex"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	debugLog *os.File
)

func debugInit(path string) {
	var err error
	debugLog, err = os.Create(path)
	if err != nil {
		exitf("creating debug log: %v", err)
	}
}

func debugf(format string, v ...interface{}) {
	if debugLog != nil {
		msg := fmt.Sprintf(format, v...)
		log.Infof(format, v...)
		if len(msg) > 0 && msg[len(msg)-1] != '\n' {
			fmt.Fprintln(debugLog, msg)
		} else {
			fmt.Fprint(debugLog, msg)
		}
	}
}

func exit(code int) {
	if ostate != nil {
		terminal.Restore(0, ostate)
	}
	log.Errorf("exit(%d)", code)
	if code != 0 {
		// Dump out the state of all our muticies
		var buf bytes.Buffer
		mutex.Dump(&buf)
		log.Errorf("Mutex Dump:\n%s", buf.String())

		// This is our thread
		log.DumpStack()
		// This is all the goroutines
		log.DumpGoroutines()
	}
	os.Exit(code)
}

func exitf(format string, v ...interface{}) {
	log.DepthErrorf(1, format, v...)
	printf(format, v...)
	exit(1)
}

func warnf(format string, v ...interface{}) {
	log.DepthWarnf(1, format, v...)
	printf(format, v...)
}

func printf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	if len(msg) > 0 && msg[len(msg)-1] != '\n' {
		fmt.Fprintln(os.Stderr, msg)
	} else {
		fmt.Fprint(os.Stderr, msg)
	}
}

// parseEscapeChar parses the string echar that should represent a single
// character and returns that character and true.  The string may be "" in which
// case 0, true is returned, a single character, a two byte sequence starting
// with '^' (control characters) or any string that is parseable by
// strconv.Unquote (parseEscapeChar adds the leading and trailing " if needed)
// and results in a single charcter (e.g., "\a" or "\176").  0, false is
// returned if echar does not unquote to a single character or is otherwise
// invalid.
func parseEscapeChar(echar string) (byte, bool) {
	switch len(echar) {
	case 0:
		return 0, true
	case 1:
		return echar[0], true
	case 2:
		switch echar[0] {
		case '^':
			return echar[1] & 037, true
		}
		if echar == `\0` {
			return 0, true
		}
	}
	if echar[0] != '"' {
		echar = `"` + echar + `"`
	}
	s, _ := strconv.Unquote(echar)
	if len(s) != 1 {
		return 0, false
	}
	return s[0], true
}

func printEscape(c byte) string {
	if c < ' ' {
		return "^" + string(c+'@')
	}
	if c <= '~' {
		return string(c)
	}
	s := strconv.QuoteRune(rune(c))
	return s[1 : len(s)-1]
}

func encodeSize(rows, cols int) []byte {
	var buf [4]byte
	buf[0] = byte(rows >> 8)
	buf[1] = byte(rows)
	buf[2] = byte(cols >> 8)
	buf[3] = byte(cols)
	return buf[:]
}

func decodeSize(buf []byte) (int, int) {
	return (int(buf[0]) << 8) | int(buf[1]), (int(buf[2]) << 8) | int(buf[3])
}
