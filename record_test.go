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
	"strings"
	"testing"
)

// ropenQuery is how ropen asks an iPadSSH terminal for its address.
const ropenQuery = "\033]7717;ropen\007"

// recordOf returns what the record filters record of in, written all at
// once if whole is true and otherwise a byte at a time.
func recordOf(in string, whole bool) (normal, alt string) {
	e := NewEscapeBuffer(1 << 20)
	addRecordFilters(e)
	if whole {
		e.Write([]byte(in))
	} else {
		for i := 0; i < len(in); i++ {
			e.Write([]byte{in[i]})
		}
	}
	e.Flush()
	return string(e.normal), string(e.alt)
}

var recordFilterTests = []struct {
	name string
	in   string
	want string // recorded on the normal screen
}{
	// Queries are dropped.
	{"DA1", "a\033[cb\033[0cc", "abc"},
	{"DA2", "a\033[>cb\033[>0cc", "abc"},
	{"DA3", "a\033[=cb", "ab"},
	{"DECID", "a\033Zb", "ab"},
	{"ENQ", "a\005b", "ab"},
	{"DSR status", "a\033[5nb", "ab"},
	{"CPR", "a\033[6nb", "ab"},
	{"DECXCPR", "a\033[?6nb", "ab"},
	{"DEC DSR", "a\033[?15nb\033[?996nc", "abc"},
	{"DECRQM ANSI", "a\033[4$pb", "ab"},
	{"DECRQM private", "a\033[?2026$pb", "ab"},
	{"XTVERSION", "a\033[>qb\033[>0qc", "abc"},
	{"kitty keyboard query", "a\033[?ub", "ab"},
	{"DECRQTSR", "a\033[1$ub", "ab"},
	{"DECRQUPSS", "a\033[&ub", "ab"},
	{"DECRQPSR", "a\033[1$wb", "ab"},
	{"DECREQTPARM", "a\033[0xb", "ab"},
	{"DECRQCRA", "a\033[1;1;1;1;2;2*yb", "ab"},
	{"DECRQLP", "a\033[1'|b", "ab"},
	{"XTREPORTSGR", "a\033[1;1;1;1#|b", "ab"},
	{"XTREPORTCOLORS", "a\033[#Rb", "ab"},
	{"DECRQDE", "a\033[\"vb", "ab"},
	{"XTQMODKEYS", "a\033[?4mb", "ab"},
	{"XTSMGRAPHICS read", "a\033[?1;1Sb\033[?2;4Sc", "abc"},
	{"report window", "a\033[11tb\033[14tc\033[18td\033[21te", "abcde"},
	{"move window", "a\033[3;10;20tb", "ab"},
	{"resize window", "a\033[8;24;80tb\033[4;100;200tc", "abc"},
	{"iconify window", "a\033[2tb", "ab"},
	{"DECSLPP", "a\033[30tb", "ab"},
	{"DECRQSS", "a\033P$qm\033\\b", "ab"},
	{"XTGETTCAP", "a\033P+q544e\033\\b", "ab"},
	{"OSC 4 query", "a\033]4;1;?\007b", "ab"},
	{"OSC 5 query", "a\033]5;0;?\033\\b", "ab"},
	{"OSC 11 query", "a\033]11;?\007b", "ab"},
	{"OSC 10,11 query", "a\033]10;?;?\007b", "ab"},
	{"OSC 50 query", "a\033]50;?\007b", "ab"},
	{"XTQALLOWED", "a\033]60\007b\033]61;allowFontOps\007c", "abc"},
	{"iTerm2 reports", "a\033]1337;ReportCellSize\007b\033]1337;ReportVariable=dXNlcg==\007c", "abc"},
	{"OSC 52", "a\033]52;c;aGk=\007b\033]52;c;?\033\\c", "abc"},
	{"ropen BEL", "a" + ropenQuery + "b", "ab"},
	{"ropen ST", "a\033]7717;ropen\033\\b", "ab"},

	// Settings that share a sequence with a query keep the setting.
	{"OSC 4 mixed", "a\033]4;1;?;2;red\007b", "a\033]4;2;red\007b"},
	{"OSC 4 mixed ST", "a\033]4;1;red;2;?;3;blue\033\\b", "a\033]4;1;red;3;blue\033\\b"},
	{"OSC 10 mixed", "a\033]10;?;blue\007b", "a\033]11;blue\007b"},
	{"OSC 10 mixed, two kept", "a\033]10;red;?;blue\007b", "a\033]10;red\007\033]12;blue\007b"},
	{"OSC 10 mixed, ended by ESC", "a\033]10;?;blue\033[0mb", "a\033]11;blue\033[0mb"},

	// Everything else is recorded as it is.
	{"SGR", "a\033[1;31mb\033[0mc", "a\033[1;31mb\033[0mc"},
	{"cursor style", "a\033[2 qb", "a\033[2 qb"},
	{"DECLL", "a\033[1qb", "a\033[1qb"},
	{"SCORC", "a\033[ub", "a\033[ub"},
	{"kitty keyboard push", "a\033[>1ub\033[<uc\033[=1;1ud", "a\033[>1ub\033[<uc\033[=1;1ud"},
	{"title stack", "a\033[22;0tb\033[23;0tc", "a\033[22;0tb\033[23;0tc"},
	{"empty t", "a\033[tb", "a\033[tb"},
	{"DSR other", "a\033[0nb", "a\033[0nb"},
	{"modify keys", "a\033[>4;1mb", "a\033[>4;1mb"},
	{"DECSTR", "a\033[!pb", "a\033[!pb"},
	{"XTSMGRAPHICS set", "a\033[?1;3;256Sb", "a\033[?1;3;256Sb"},
	{"Linux cursor", "a\033[?2cb", "a\033[?2cb"},
	{"CSI with C0 inside", "a\033[1\r5nb", "a\033[1\r5nb"},
	{"CSI aborted by CAN", "a\033[6\030b", "a\033[6\030b"},
	{"CSI ended by ESC", "a\033[6\033[1mb", "a\033[6\033[1mb"},
	{"CSI ended by UTF-8", "a\033[6é", "a\033[6é"},
	{"OSC title", "a\033]0;title\007b", "a\033]0;title\007b"},
	{"OSC 4 set", "a\033]4;1;red\007b", "a\033]4;1;red\007b"},
	{"OSC 10 set", "a\033]10;red;blue\007b", "a\033]10;red;blue\007b"},
	{"OSC 4 malformed", "a\033]4;1;?;2\007b", "a\033]4;1;?;2\007b"},
	{"OSC 7", "a\033]7;file:///tmp\033\\b", "a\033]7;file:///tmp\033\\b"},
	{"OSC 8", "a\033]8;;http://x\007l\033]8;;\007b", "a\033]8;;http://x\007l\033]8;;\007b"},
	{"OSC 520", "a\033]520;x\007b", "a\033]520;x\007b"},
	{"OSC 50 set", "a\033]50;fixed\007b", "a\033]50;fixed\007b"},
	{"OSC 7717 other", "a\033]7717;other\007b", "a\033]7717;other\007b"},
	{"iTerm2 other", "a\033]1337;SetMark\007b", "a\033]1337;SetMark\007b"},
	{"sixel", "a\033Pq#0;2;0;0;0~-\033\\b", "a\033Pq#0;2;0;0;0~-\033\\b"},
	{"DECUDK", "a\033P1;1|17/7465\033\\b", "a\033P1;1|17/7465\033\\b"},
	{"DCS ended by ESC", "a\033P$\033[1mb", "a\033P$\033[1mb"},
	{"scroll up", "a\033[3Sb", "a\033[3Sb"},
	{"XTSMTITLE", "a\033[>1tb", "a\033[>1tb"},
	{"n with intermediate", "a\033[6 nb", "a\033[6 nb"},
	{"cursor position", "a\033[2;3Hb", "a\033[2;3Hb"},
	{"empty OSC", "a\033]\033[1mb", "a\033]\033[1mb"},
	{"lone ESC", "a\033b", "a\033b"},
	{"ESC ESC", "a\033\033[cb", "a\033b"},
	{"text", "plain text\r\n", "plain text\r\n"},
	{"UTF-8", "héllo ☺", "héllo ☺"},
}

func TestRecordFilters(t *testing.T) {
	for _, tt := range recordFilterTests {
		for _, whole := range []bool{true, false} {
			name := tt.name
			if !whole {
				name += "/bytewise"
			}
			t.Run(name, func(t *testing.T) {
				if got, _ := recordOf(tt.in, whole); got != tt.want {
					t.Errorf("recorded %q, want %q", got, tt.want)
				}
			})
		}
	}
}

func TestRecordScreens(t *testing.T) {
	for _, tt := range []struct {
		name        string
		in          string
		normal, alt string
	}{
		{"switch", "a" + scasb + "b" + nsbrc + "c", "ac", "b"},
		{"erase saved on normal", "a" + edsaved + "b", nsbrc + edall + "b", ""},
		{"erase saved on alt", scasb + "a" + edsaved + "b", "", "ab"},
		{"erase all on alt", scasb + "a" + edall + "b", "", home + edall + "b"},
		{"erase all on normal", "a" + edall + "b", "ab", ""},
		{"query on alt", scasb + "a\033[cb", "", "ab"},
	} {
		for _, whole := range []bool{true, false} {
			name := tt.name
			if !whole {
				name += "/bytewise"
			}
			t.Run(name, func(t *testing.T) {
				normal, alt := recordOf(tt.in, whole)
				if normal != tt.normal || alt != tt.alt {
					t.Errorf("recorded %q, %q; want %q, %q", normal, alt, tt.normal, tt.alt)
				}
			})
		}
	}
}

func TestRecordLongStrings(t *testing.T) {
	long := strings.Repeat("QUJD", stringLimit)
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{"long OSC 52", "a\033]52;c;" + long + "\007b", "ab"},
		{"long OSC 52, ended by ESC", "a\033]52;c;" + long + "\033[1mb", "a\033[1mb"},
		{"long title", "a\033]0;" + long + "\033\\b", "a\033]0;" + long + "\033\\b"},
		{"long title, ended by ESC", "a\033]0;" + long + "\033[1mb", "a\033]0;" + long + "\033[1mb"},
		{"long sixel", "a\033Pq" + long + "\033\\b", "a\033Pq" + long + "\033\\b"},
		{"long DECRQSS", "a\033P$q" + long + "\033\\b", "ab"},
		{"long CSI", "a\033[" + strings.Repeat("1;", csiLimit) + "mb", "a\033[" + strings.Repeat("1;", csiLimit) + "mb"},
	} {
		for _, whole := range []bool{true, false} {
			name := tt.name
			if !whole {
				name += "/bytewise"
			}
			t.Run(name, func(t *testing.T) {
				if got, _ := recordOf(tt.in, whole); got != tt.want {
					t.Errorf("recorded %d bytes, want %d", len(got), len(tt.want))
				}
			})
		}
	}
}

// TestRecordLongStringHoldsLittle checks that an OSC 52 too long to hold is
// not held while it is open.
func TestRecordLongStringHoldsLittle(t *testing.T) {
	e := NewEscapeBuffer(1 << 20)
	addRecordFilters(e)
	e.Write([]byte("\033]52;c;"))
	chunk := []byte(strings.Repeat("QUJD", 1024))
	for i := 0; i < 100; i++ {
		e.Write(chunk)
		if e.inseq == nil {
			t.Fatal("sequence ended early")
		}
		if n := len(e.inseq.seen); n > stringLimit+len(chunk) {
			t.Fatalf("after %d writes the open sequence holds %d bytes", i+1, n)
		}
	}
	if n := len(e.inseq.seen); n > 1 {
		t.Errorf("open sequence holds %d bytes, want at most 1", n)
	}
}

// TestRecordFiltersSplit writes each recordFilterTests input as two writes,
// split at every place, all followed by a second copy of the input.
func TestRecordFiltersSplit(t *testing.T) {
	for _, tt := range recordFilterTests {
		in := tt.in + tt.in
		for i := 1; i < len(in); i++ {
			e := NewEscapeBuffer(1 << 20)
			addRecordFilters(e)
			e.Write([]byte(in[:i]))
			e.Write([]byte(in[i:]))
			e.Flush()
			if got, want := string(e.normal), tt.want+tt.want; got != want {
				t.Errorf("%s split at %d: recorded %q, want %q", tt.name, i, got, want)
			}
		}
	}
}

func TestCSIParam(t *testing.T) {
	for _, tt := range []struct {
		in   string
		i    int
		want int
	}{
		{"\033[m", 0, -1},
		{"\033[5m", 0, 5},
		{"\033[5m", 1, -1},
		{"\033[1;;3m", 1, -1},
		{"\033[1;;3m", 2, 3},
		{"\033[38:5:12m", 0, -1},
		{"\033[1234567890m", 0, -1},
		{"\033[?25h", 0, 25},
	} {
		c, ok := parseCSI([]byte(tt.in))
		if !ok {
			t.Errorf("parseCSI(%q) failed", tt.in)
			continue
		}
		if got := c.param(tt.i); got != tt.want {
			t.Errorf("param(%d) of %q = %d, want %d", tt.i, tt.in, got, tt.want)
		}
	}
	for _, in := range []string{"\033[", "\033[1", "\033[1\rm"} {
		if _, ok := parseCSI([]byte(in)); ok {
			t.Errorf("parseCSI(%q) succeeded", in)
		}
	}
}
