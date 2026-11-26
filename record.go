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
	"strconv"
	"strings"
)

// What a shell writes is passed unchanged to the attached clients and also
// recorded, and the record is replayed to each client that attaches.  Some
// sequences must not be replayed:
//
//   - Queries.  The terminal answers a query by typing the answer into the
//     input, so a replayed query types a stale answer at whatever is reading
//     input when the client attaches.  These are device attributes, status
//     and cursor position reports, mode, version, keyboard, window and
//     colour reports, termcap and status-string requests, and the like.
//   - Window manipulation (CSI t): replaying it would move, resize or
//     iconify the window to suit long-gone output.
//   - OSC 52, which sets a clipboard (replaying it sets the clipboard to a
//     stale value) or queries it.
//   - Pty's own ESC [ z, and the sequences in seqRules (the ropen query,
//     and any named in config.yaml).
//
// The record filters below drop these from the record.  Anything they do
// not recognize is recorded as it arrived.  A sequence that sets something
// and queries something else at the same time (OSC 4 and OSC 10 through 19
// can) is recorded with just the setting.
//
// The filters also keep track of the switches between the normal and
// alternate screens, and of the erases that let pty discard what has been
// recorded for a screen.

const (
	csiIntro = "\033["
	oscIntro = "\033]"
	dcsIntro = "\033P"
	decid    = "\033Z" // Identify Terminal: answered as Primary DA
	enq      = "\005"  // Enquiry: answered with the answerback string

	// The most of an OSC or DCS held while waiting for it to end.  A
	// longer one (an image, say) is recorded as it arrives, or, if it is
	// an OSC 52, dropped as it arrives.  Every query is much shorter.
	stringLimit = 4096

	// The most of a CSI held while waiting for it to end.  A longer
	// one is not a sequence any terminal understands.
	csiLimit = 256
)

var (
	// An OSC ends the way a terminal ends one: with BEL or ST (ESC \),
	// aborted by CAN or SUB, or just before any other ESC, which starts
	// the next escape sequence.  A DCS is the same except that BEL does
	// not end it.
	oscTerms   = []string{"\007", "\033\\", "\030", "\032"}
	dcsTerms   = []string{"\033\\", "\030", "\032"}
	stringEnds = "\033"

	// A CSI ends with its final byte (0x40 through 0x7e), is aborted by
	// CAN or SUB, or ends just before an ESC or a byte that is not ASCII.
	csiTerms, csiEnds = csiEnding()
)

func csiEnding() ([]string, string) {
	terms := []string{"\030", "\032"}
	for c := 0x40; c <= 0x7e; c++ {
		terms = append(terms, string([]byte{byte(c)}))
	}
	ends := []byte{'\033'}
	for c := 0x80; c <= 0xff; c++ {
		ends = append(ends, byte(c))
	}
	return terms, string(ends)
}

// addRecordFilters registers the record filters with e.
func addRecordFilters(e *EscapeBuffer) {
	e.AddFilterSequence(csiIntro, csiTerms, csiEnds, csiLimit, recordCSI)
	e.AddFilterSequence(oscIntro, oscTerms, stringEnds, stringLimit, recordOSC)
	e.AddFilterSequence(dcsIntro, dcsTerms, stringEnds, stringLimit, recordDCS)
	e.AddSequence(decid, func(e *EscapeBuffer) bool {
		e.expect("CSI ?c")
		return false
	})
	e.AddSequence(enq, func(*EscapeBuffer) bool { return false })
	for _, r := range seqRules {
		if r.kind == 0 {
			r := r
			e.AddFilterSequence(string(r.start), r.end, "", r.limit, func(e *EscapeBuffer, raw []byte, complete bool) ([]byte, bool) {
				if !complete || r.tooLong(raw) {
					// Too long: it is not r after all.
					return raw, true
				}
				e.expect(r.key)
				return nil, false
			})
		}
	}
}

// A csiSeq is a parsed control sequence: ESC [ private params inter final.
// Params and inter are slices of the sequence's bytes.
type csiSeq struct {
	private byte   // one of < = > ?, or 0
	params  []byte // parameter bytes, 0x30 through 0x3f
	inter   []byte // intermediate bytes, 0x20 through 0x2f
	final   byte
}

// parseCSI parses raw, which starts with ESC [.  It returns false if raw is
// not a complete, well formed control sequence.
func parseCSI(raw []byte) (csiSeq, bool) {
	var c csiSeq
	b := raw[len(csiIntro):]
	if len(b) == 0 || b[len(b)-1] < 0x40 || b[len(b)-1] > 0x7e {
		return c, false
	}
	c.final = b[len(b)-1]
	b = b[:len(b)-1]
	if len(b) > 0 && b[0] >= '<' && b[0] <= '?' {
		c.private = b[0]
		b = b[1:]
	}
	i := 0
	for i < len(b) && b[i] >= 0x30 && b[i] <= 0x3f {
		i++
	}
	c.params, c.inter = b[:i], b[i:]
	for _, x := range c.inter {
		if x < 0x20 || x > 0x2f {
			return c, false
		}
	}
	return c, true
}

// param returns the i'th parameter of c, or -1 if it is empty, missing or
// not a number.
func (c csiSeq) param(i int) int {
	p := c.params
	for ; i > 0; i-- {
		x := bytes.IndexByte(p, ';')
		if x < 0 {
			return -1
		}
		p = p[x+1:]
	}
	if x := bytes.IndexByte(p, ';'); x >= 0 {
		p = p[:x]
	}
	if len(p) == 0 || len(p) > 9 {
		return -1
	}
	n := 0
	for _, d := range p {
		if d < '0' || d > '9' {
			return -1
		}
		n = n*10 + int(d-'0')
	}
	return n
}

// is reports whether c's intermediate bytes are inter.
func (c csiSeq) is(inter string) bool {
	return string(c.inter) == inter
}

// classify reports whether c is dropped from the record, which it is if it
// is a query or a window manipulation.  For a query whose answer pty can
// recognize, answer is the answer's key (see answerKey); otherwise it is "".
func (c csiSeq) classify() (drop bool, answer string) {
	none := c.is("")
	plain := c.private == 0 && none
	switch c.final {
	case 'c': // Primary, Secondary and Tertiary DA
		if none {
			switch c.private {
			case 0:
				return true, "CSI ?c"
			case '>':
				return true, "CSI >c"
			case '=':
				return true, "DCS !|"
			}
		}
	case 'n': // DSR, CPR, and the DEC-specific DSRs
		if !none {
			break
		}
		p := c.param(0)
		switch {
		case c.private == '?' && p == 6: // DECXCPR
			return true, "CSI ?R"
		case c.private == '?':
			return true, "CSI ?n"
		case c.private == 0 && p == 5:
			return true, "CSI n"
		case c.private == 0 && p == 6: // CPR
			return true, "CSI R"
		}
	case 'p': // DECRQM, ANSI and DEC private
		if c.is("$") {
			if c.private == '?' {
				return true, "CSI ?$y"
			}
			return true, "CSI $y"
		}
	case 'q': // XTVERSION
		if c.private == '>' && none {
			return true, "DCS >|"
		}
	case 'u': // kitty keyboard query, DECRQTSR, DECRQUPSS
		switch {
		case c.private == '?' && none:
			return true, "CSI ?u"
		case c.is("$"):
			return true, "DCS $s"
		case c.is("&"):
			return true, "DCS !u"
		}
	case 'w': // DECRQPSR
		if c.is("$") {
			return true, "DCS $u"
		}
	case 'x': // DECREQTPARM
		if plain {
			return true, "CSI x"
		}
	case 'y': // DECRQCRA
		if c.is("*") {
			return true, "DCS !~"
		}
	case '|': // DECRQLP, XTREPORTSGR
		switch {
		case c.is("'"):
			return true, "CSI &w"
		case c.is("#"):
			return true, "CSI m"
		}
	case 'R': // XTREPORTCOLORS
		if c.is("#") {
			return true, ""
		}
	case 'v': // DECRQDE
		if c.is("\"") {
			return true, "CSI \"w"
		}
	case 'm': // XTQMODKEYS
		if c.private == '?' && none {
			return true, "CSI >m"
		}
	case 'S': // XTSMGRAPHICS read (1) and read maximum (4)
		if c.private == '?' && none {
			if p := c.param(1); p == 1 || p == 4 {
				return true, "CSI ?S"
			}
		}
	case 't': // XTWINOPS: 1-21 manipulate or report, 24+ resize (DECSLPP)
		if !plain {
			break
		}
		switch p := c.param(0); {
		case p == 11, p >= 13 && p <= 19 && p != 17:
			return true, "CSI t"
		case p == 20:
			return true, "OSC L"
		case p == 21:
			return true, "OSC l"
		case p >= 1 && p <= 21, p >= 24:
			return true, ""
		}
	}
	return false, ""
}

// recordCSI is the FilterFunc for control sequences.
func recordCSI(e *EscapeBuffer, raw []byte, complete bool) ([]byte, bool) {
	if !complete {
		return raw, true
	}
	switch string(raw) {
	case sendSSH:
		return nil, false
	case scasb:
		e.inalt = true
		return nil, false
	case nsbrc:
		e.inalt = false
		return nil, false
	case edsaved:
		if !e.inalt {
			e.normal = e.normal[:0]
			e.normal = append(e.normal, []byte(nsbrc)...)
			e.normal = append(e.normal, []byte(edall)...)
		}
		return nil, false
	case edall:
		if e.inalt {
			e.alt = e.alt[:0]
			e.alt = append(e.alt, []byte(home)...)
			e.alt = append(e.alt, []byte(edall)...)
		}
		return nil, false
	}
	if r := matchRule('[', raw); r != nil {
		e.expect(r.key)
		return nil, false
	}
	if c, ok := parseCSI(raw); ok {
		if drop, answer := c.classify(); drop {
			e.expect(answer)
			return nil, false
		}
	}
	return raw, false
}

// stringTerm returns the length of the terminator at the end of body, the
// bytes of a complete OSC or DCS after its introducer.  The terminator is
// one of oscTerms, or nothing if the sequence ended before an ESC.  None of
// those bytes can appear in body other than as its terminator.
func stringTerm(body []byte) int {
	switch {
	case bytes.HasSuffix(body, []byte("\033\\")):
		return 2
	case len(body) == 0:
		return 0
	}
	switch body[len(body)-1] {
	case '\007', '\030', '\032':
		return 1
	}
	return 0
}

// recordOSC is the FilterFunc for operating system commands.
func recordOSC(e *EscapeBuffer, raw []byte, complete bool) ([]byte, bool) {
	body := raw[len(oscIntro):]
	var term []byte
	if complete {
		tl := stringTerm(body)
		body, term = body[:len(body)-tl], body[len(body)-tl:]
	}
	ps, rest, _ := strings.Cut(string(body), ";")
	r := matchRule(']', raw)
	if !complete {
		// Too long to be a query; only an OSC 52 or one in
		// seqRules is dropped.
		return raw, ps != "52" && r == nil
	}
	if r != nil {
		e.expect(r.key)
		return nil, false
	}
	switch ps {
	case "52": // clipboard
		if _, pd, _ := strings.Cut(rest, ";"); pd == "?" {
			e.expect("OSC 52")
		}
		return nil, false
	case "4", "5": // colour number (c;spec pairs)
		out := oscColorPairs(raw, ps, rest, term)
		if len(out) != len(raw) {
			e.expect("OSC " + ps)
		}
		return out, false
	case "10", "11", "12", "13", "14", "15", "16", "17", "18", "19":
		n, _ := strconv.Atoi(ps)
		for i, v := range strings.Split(rest, ";") {
			if v == "?" {
				e.expect("OSC " + strconv.Itoa(n+i))
			}
		}
		return oscDynamicColors(raw, ps, rest, term), false
	case "50": // font
		if strings.HasPrefix(rest, "?") {
			e.expect("OSC 50")
			return nil, false
		}
	case "60", "61": // XTQALLOWED, XTQDISALLOWED
		e.expect("OSC " + ps)
		return nil, false
	case "1337": // iTerm2
		if strings.HasPrefix(rest, "ReportCellSize") || strings.HasPrefix(rest, "ReportVariable=") {
			e.expect("OSC 1337")
			return nil, false
		}
	}
	return raw, false
}

// oscColorPairs returns what to record for OSC ps;rest, where ps is 4 or 5
// and rest is c;spec pairs: raw without the pairs whose spec is "?".
func oscColorPairs(raw []byte, ps, rest string, term []byte) []byte {
	parts := strings.Split(rest, ";")
	if len(parts)%2 != 0 {
		return raw
	}
	var kept []string
	for i := 0; i < len(parts); i += 2 {
		if parts[i+1] != "?" {
			kept = append(kept, parts[i], parts[i+1])
		}
	}
	switch len(kept) {
	case len(parts):
		return raw
	case 0:
		return nil
	}
	out := []byte(oscIntro + ps + ";" + strings.Join(kept, ";"))
	return append(out, term...)
}

// oscDynamicColors returns what to record for OSC ps;rest, where ps is 10
// through 19 and rest is a list of values for colours ps, ps+1 and so on:
// raw without the values that are "?".  Since dropping a value would move
// the ones after it to other colours, each value that is kept gets its own
// OSC.
func oscDynamicColors(raw []byte, ps, rest string, term []byte) []byte {
	n, _ := strconv.Atoi(ps)
	vals := strings.Split(rest, ";")
	var out []byte
	dropped := false
	for i, v := range vals {
		if v == "?" {
			dropped = true
			continue
		}
		out = append(out, oscIntro+strconv.Itoa(n+i)+";"+v...)
		out = append(out, term...)
	}
	if !dropped {
		return raw
	}
	return out
}

// recordDCS is the FilterFunc for device control strings.  It drops
// DECRQSS (DCS $ q) and XTGETTCAP (DCS + q).
func recordDCS(e *EscapeBuffer, raw []byte, complete bool) ([]byte, bool) {
	if r := matchRule('P', raw); r != nil {
		if complete {
			e.expect(r.key)
		}
		return nil, false
	}
	b := raw[len(dcsIntro):]
	i := 0
	for i < len(b) && b[i] >= 0x30 && b[i] <= 0x3f {
		i++
	}
	j := i
	for j < len(b) && b[j] >= 0x20 && b[j] <= 0x2f {
		j++
	}
	if j < len(b) && b[j] == 'q' {
		switch string(b[i:j]) {
		case "$":
			e.expect("DCS $r")
			return nil, false
		case "+":
			e.expect("DCS +r")
			return nil, false
		}
	}
	return raw, true
}
