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
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// queryAnswers pairs each query the record filters drop with an answer a
// terminal sends to it.
var queryAnswers = []struct {
	name, query, answer string
}{
	{"DA1", "\033[c", "\033[?62;22c"},
	{"DA1 0", "\033[0c", "\033[?62;22c"},
	{"DECID", "\033Z", "\033[?1;2c"},
	{"DA2", "\033[>c", "\033[>1;10;0c"},
	{"DA3", "\033[=c", "\033P!|00000000\033\\"},
	{"DSR", "\033[5n", "\033[0n"},
	{"CPR", "\033[6n", "\033[12;40R"},
	{"DECXCPR", "\033[?6n", "\033[?12;40;1R"},
	{"colour scheme", "\033[?996n", "\033[?997;1n"},
	{"DECRQM ANSI", "\033[4$p", "\033[4;2$y"},
	{"DECRQM private", "\033[?2026$p", "\033[?2026;2$y"},
	{"XTVERSION", "\033[>q", "\033P>|xterm(370)\033\\"},
	{"kitty keyboard", "\033[?u", "\033[?1u"},
	{"DECRQTSR", "\033[1$u", "\033P1$s\033\\"},
	{"DECRQUPSS", "\033[&u", "\033P0!u%5\033\\"},
	{"DECRQPSR", "\033[1$w", "\033P1$u1;1;1;@;@;@;0;2;@;BB%5\033\\"},
	{"DECREQTPARM", "\033[0x", "\033[2;1;1;112;112;1;0x"},
	{"DECRQCRA", "\033[1;1;1;1;2;2*y", "\033P1!~ABCD\033\\"},
	{"DECRQLP", "\033[1'|", "\033[1;0;10;20;1&w"},
	{"XTREPORTSGR", "\033[1;1;1;1#|", "\033[0m"},
	{"DECRQDE", "\033[\"v", "\033[24;80;1;1;1\"w"},
	{"XTQMODKEYS", "\033[?4m", "\033[>4;2m"},
	{"XTSMGRAPHICS", "\033[?1;1S", "\033[?1;0;256S"},
	{"window state", "\033[11t", "\033[1t"},
	{"window pixels", "\033[14t", "\033[4;600;800t"},
	{"text size", "\033[18t", "\033[8;24;80t"},
	{"icon label", "\033[20t", "\033]LIcon\033\\"},
	{"title", "\033[21t", "\033]lTitle\033\\"},
	{"DECRQSS", "\033P$qm\033\\", "\033P1$r0m\033\\"},
	{"XTGETTCAP", "\033P+q544e\033\\", "\033P1+r544e=787465726d\033\\"},
	{"OSC 4", "\033]4;1;?\007", "\033]4;1;rgb:cdcd/0000/0000\007"},
	{"OSC 5", "\033]5;0;?\007", "\033]5;0;rgb:0000/0000/0000\033\\"},
	{"OSC 11", "\033]11;?\033\\", "\033]11;rgb:0000/0000/0000\033\\"},
	{"OSC 10 for 11", "\033]10;?;?\007", "\033]11;rgb:0000/0000/0000\007"},
	{"OSC 50", "\033]50;?\007", "\033]50;fixed\007"},
	{"OSC 52", "\033]52;c;?\007", "\033]52;c;aGk=\007"},
	{"XTQALLOWED", "\033]60\007", "\033]60;allowColorOps,allowFontOps\007"},
	{"iTerm2", "\033]1337;ReportCellSize\007", "\033]1337;ReportCellSize=17.0;8.0;2.0\007"},
	{"ropen", ropenQuery, "#ROPEN 10.0.0.1:1234/abcd NEPOR#"},
}

func newAnswerShell(t *testing.T) *Shell {
	t.Helper()
	t.Cleanup(withTempHome(t))
	sess := MakeSession("answers", "")
	t.Cleanup(sess.Remove)
	return NewShell(sess)
}

func waitKeys(s *Shell) []string {
	var keys []string
	for k := range s.waits {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestQueryAnswerKeys(t *testing.T) {
	for _, tt := range queryAnswers {
		t.Run(tt.name, func(t *testing.T) {
			s := newAnswerShell(t)
			s.eb.Write([]byte(tt.query))
			n, key, complete := scanAnswer([]byte(tt.answer))
			if n != len(tt.answer) || !complete {
				t.Fatalf("scanAnswer(%q) = %d, %q, %v", tt.answer, n, key, complete)
			}
			if s.waits[key] == nil {
				t.Errorf("answer key %q, expected %q", key, waitKeys(s))
			}
		})
	}
}

func TestNoAnswerExpected(t *testing.T) {
	for _, out := range []string{
		"\033[1;31mtext\033[0m",
		"\033[8;24;80t", // dropped, but not a query
		"\033[3;10;20t",
		"\033[12t",
		"\033[#R",
		"\033]52;c;aGk=\007", // a set, not a query
		"\033]4;1;red\007",
		"\033]10;red\007",
		"\033]50;fixed\007",
		"\033]1337;SetMark\007",
		"\033]7717;other\007",
		"\033Pq#0~\033\\",
	} {
		s := newAnswerShell(t)
		s.eb.Write([]byte(out))
		if len(s.waits) != 0 {
			t.Errorf("after %q, expected %q", out, waitKeys(s))
		}
	}
}

func TestScanAnswer(t *testing.T) {
	for _, tt := range []struct {
		in       string
		n        int
		key      string
		complete bool
	}{
		{"\033", 1, "", false},
		{"\033[", 2, "", false},
		{"\033[12;4", 6, "", false},
		{"\033[A", 3, "CSI A", true},
		{"\033[1;5Rxyz", 6, "CSI R", true},
		{"\033[1\030x", 4, "", true},
		{"\033[1\033[A", 3, "", true},
		{"\033[1é", 3, "", true},
		{"\033[1\rA", 5, "", true},
		{"\033x", 2, "", true},
		{"\033]11;rgb:0/0/0", 14, "", false},
		{"\033]11;rgb:0/0/0\033", 15, "", false},
		{"\033]11;rgb:0/0/0\033\\x", 16, "OSC 11", true},
		{"\033]11;x\033[A", 6, "", true},
		{"\033]11;x\030", 7, "", true},
		{"\033]\007", 3, "OSC ", true},
		{"\033P1$r0m\007\033\\", 10, "DCS $r", true},
		{"\033P1$\033\\", 6, "", true},
		{"\033P\033\\", 4, "", true},
		{"#", 1, "", false},
		{"#RO", 3, "", false},
		{"#x", 1, "", true},
		{"#ROPEX", 1, "", true},
		{"#ROPENX x", 1, "", true},
		{"#ROPEN 1.2.3.4:5/t", 18, "", false},
		{"#ROPEN 1.2.3.4:5/t NEPOR#x", 25, ropenAnswer, true},
	} {
		n, key, complete := scanAnswer([]byte(tt.in))
		if n != tt.n || key != tt.key || complete != tt.complete {
			t.Errorf("scanAnswer(%q) = %d, %q, %v; want %d, %q, %v", tt.in, n, key, complete, tt.n, tt.key, tt.complete)
		}
	}
}

// filterAnswers runs the reads in through f, with keys expected and kept as
// given, and returns what came out of each read and whether it was typed.
func filterAnswers(f *answerFilter, expect, keep map[string]bool, reads ...string) []string {
	var outs []string
	for _, in := range reads {
		out, typed := f.Filter([]byte(in), func(key string) (bool, bool) {
			return expect[key], keep[key]
		})
		outs = append(outs, fmt.Sprintf("%q %v", out, typed))
	}
	return outs
}

func TestAnswerFilter(t *testing.T) {
	cpr := map[string]bool{"CSI R": true}
	ropen := map[string]bool{ropenAnswer: true}
	none := map[string]bool{}
	for _, tt := range []struct {
		name         string
		expect, keep map[string]bool
		reads        []string
		want         []string
	}{
		{"dropped", cpr, none, []string{"\033[12;40R"}, []string{`"" false`}},
		{"kept", cpr, cpr, []string{"\033[12;40R"}, []string{`"\x1b[12;40R" false`}},
		{"not expected", none, none, []string{"\033[12;40R"}, []string{`"\x1b[12;40R" true`}},
		{"typed around", cpr, none, []string{"a\033[12;40Rb"}, []string{`"ab" true`}},
		{"keys pass", cpr, none, []string{"\033[A\033[1;5A"}, []string{`"\x1b[A\x1b[1;5A" true`}},
		{"split", cpr, none, []string{"x\033[12;", "40Ry"}, []string{`"x" true`, `"y" true`}},
		{"split at ESC", cpr, none, []string{"x\033", "[12;40R"}, []string{`"x\x1b" true`, `"[12;40R" true`}},
		{"lone ESC", cpr, none, []string{"\033"}, []string{`"\x1b" true`}},
		{"held then not an answer", cpr, none, []string{"\033[1", "A"}, []string{`"" false`, `"\x1b[1A" true`}},
		{"ropen dropped", ropen, none, []string{"#ROPEN 1.2.3.4:5/t NEPOR#"}, []string{`"" false`}},
		{"ropen split", ropen, none, []string{"#ROP", "EN 1:2/t NEP", "OR#!"}, []string{`"" false`, `"" false`, `"!" true`}},
		{"hash typed", ropen, none, []string{"#x#"}, []string{`"#x" true`}},
		{"hash not expected", none, none, []string{"#ROPEN 1:2/t NEPOR#"}, []string{`"#ROPEN 1:2/t NEPOR#" true`}},
		{"too long", cpr, none, []string{"\033[" + strings.Repeat("1", answerLimit)}, []string{fmt.Sprintf("%q true", "\033["+strings.Repeat("1", answerLimit))}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var f answerFilter
			got := filterAnswers(&f, tt.expect, tt.keep, tt.reads...)
			if strings.Join(got, ", ") != strings.Join(tt.want, ", ") {
				t.Errorf("got %s\nwant %s", strings.Join(got, ", "), strings.Join(tt.want, ", "))
			}
		})
	}
}

// answerClients returns a shell with clients a and b attached.
func answerClients(t *testing.T) (s *Shell, a, b *Client) {
	s = newAnswerShell(t)
	a = NewClient(&bytes.Buffer{})
	a.SetName("a")
	b = NewClient(&bytes.Buffer{})
	b.SetName("b")
	s.Attach(a)
	s.Attach(b)
	t.Cleanup(func() {
		s.Detach(a)
		s.Detach(b)
		stopClient(a)
		stopClient(b)
	})
	return s, a, b
}

func input(s *Shell, c *Client, f *answerFilter, in string) string {
	out, typed := s.FilterInput(c, f, []byte(in))
	return fmt.Sprintf("%q %v", out, typed)
}

const cprAnswer = "\033[12;40R"

func TestFilterInputPrimary(t *testing.T) {
	s, a, b := answerClients(t)
	var fa, fb answerFilter
	s.Take(a, false)
	s.eb.Write([]byte("\033[6n"))
	if got, want := input(s, b, &fb, cprAnswer), `"" false`; got != want {
		t.Errorf("b's answer: got %s, want %s", got, want)
	}
	if got, want := input(s, a, &fa, cprAnswer), fmt.Sprintf("%q false", cprAnswer); got != want {
		t.Errorf("a's answer: got %s, want %s", got, want)
	}
	if got, want := input(s, b, &fb, "x"), `"x" true`; got != want {
		t.Errorf("b typing: got %s, want %s", got, want)
	}
}

func TestFilterInputFirstAnswer(t *testing.T) {
	s, a, b := answerClients(t)
	var fa, fb answerFilter
	s.eb.Write([]byte("\033[6n"))
	if got, want := input(s, b, &fb, cprAnswer), fmt.Sprintf("%q false", cprAnswer); got != want {
		t.Errorf("b's answer: got %s, want %s", got, want)
	}
	if got, want := input(s, a, &fa, cprAnswer), `"" false`; got != want {
		t.Errorf("a's answer: got %s, want %s", got, want)
	}
	// The next query chooses again.
	s.Take(a, false)
	s.eb.Write([]byte("\033[6n"))
	if got, want := input(s, b, &fb, cprAnswer), `"" false`; got != want {
		t.Errorf("b's second answer: got %s, want %s", got, want)
	}
}

func TestFilterInputWindow(t *testing.T) {
	defer func() { now = time.Now }()
	t0 := time.Now()
	now = func() time.Time { return t0 }
	s, a, b := answerClients(t)
	var fb answerFilter
	s.Take(a, false)
	s.eb.Write([]byte("\033[6n"))
	now = func() time.Time { return t0.Add(answerWindow - time.Millisecond) }
	if got, want := input(s, b, &fb, cprAnswer), `"" false`; got != want {
		t.Errorf("in the window: got %s, want %s", got, want)
	}
	now = func() time.Time { return t0.Add(answerWindow + time.Millisecond) }
	if got, want := input(s, b, &fb, cprAnswer), fmt.Sprintf("%q true", cprAnswer); got != want {
		t.Errorf("after the window: got %s, want %s", got, want)
	}
	if len(s.waits) != 0 {
		t.Errorf("waits after the window: %q", waitKeys(s))
	}
}

func TestFilterInputDetach(t *testing.T) {
	s, a, b := answerClients(t)
	var fb answerFilter
	s.Take(a, false)
	s.eb.Write([]byte("\033[6n\033[c"))
	input(s, a, &answerFilter{}, cprAnswer) // a is chosen for CSI R
	s.Detach(a)
	if s.primary != nil {
		t.Error("primary not cleared by Detach")
	}
	if got, want := input(s, b, &fb, cprAnswer+"\033[?62c"), fmt.Sprintf("%q false", cprAnswer+"\033[?62c"); got != want {
		t.Errorf("b's answers after a detached: got %s, want %s", got, want)
	}
}

func TestFilterInputHeldAfterWindow(t *testing.T) {
	defer func() { now = time.Now }()
	t0 := time.Now()
	now = func() time.Time { return t0 }
	s, a, b := answerClients(t)
	var fb answerFilter
	s.Take(a, false)
	s.eb.Write([]byte("\033[6n"))
	if got, want := input(s, b, &fb, "\033[1"), `"" false`; got != want {
		t.Errorf("start of a sequence: got %s, want %s", got, want)
	}
	now = func() time.Time { return t0.Add(2 * answerWindow) }
	if got, want := input(s, b, &fb, "A"), `"\x1b[1A" true`; got != want {
		t.Errorf("rest after the window: got %s, want %s", got, want)
	}
}

// A pipeClient is a client attached to a Shell through Shell.attach.
type pipeClient struct {
	mw   *MessengerWriter
	acks chan []byte
}

func attachPipeClient(t *testing.T, s *Shell, name string) *pipeClient {
	t.Helper()
	server, client := net.Pipe()
	go s.attach(server)
	pc := &pipeClient{mw: NewMessengerWriter(client), acks: make(chan []byte, 10)}
	r := NewMessengerReader(client, func(kind messageKind, msg []byte) {
		if kind == ackMessage {
			pc.acks <- append([]byte{}, msg...)
		}
	})
	go io.Copy(io.Discard, r)
	t.Cleanup(func() { client.Close() })
	pc.mw.Send(ttynameMessage, []byte(name))
	pc.sync(t)
	return pc
}

// sync waits for the server to have dealt with everything pc has sent.
func (pc *pipeClient) sync(t *testing.T) {
	t.Helper()
	pc.mw.Send(pingMessage, []byte("sync"))
	select {
	case <-pc.acks:
	case <-time.After(5 * time.Second):
		t.Fatal("no ack from the server")
	}
}

// A lockedBuffer collects what is written to the shell's pty.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *lockedBuffer) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for b.String() != want {
		if time.Now().After(deadline) {
			t.Fatalf("the program got %q, want %q", b.String(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAttachAnswers(t *testing.T) {
	s := newAnswerShell(t)
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	defer pw.Close()
	s.pty = pw
	var got lockedBuffer
	go func() {
		var buf [1024]byte
		for {
			n, err := pr.Read(buf[:])
			got.mu.Lock()
			got.buf.Write(buf[:n])
			got.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	a := attachPipeClient(t, s, "a")
	b := attachPipeClient(t, s, "b")
	primary := func() string {
		defer s.mu.Lock("test")()
		if s.primary == nil {
			return ""
		}
		return s.primary.Name()
	}

	a.mw.Write([]byte("x"))
	a.sync(t)
	got.waitFor(t, "x")
	if p := primary(); p != "a" {
		t.Fatalf("primary is %q, want a", p)
	}

	unlock := s.mu.Lock("test")
	s.eb.Write([]byte("\033[6n"))
	unlock()

	b.mw.Write([]byte(cprAnswer))
	b.sync(t)
	if p := primary(); p != "a" {
		t.Errorf("after b's answer the primary is %q, want a", p)
	}
	a.mw.Write([]byte(cprAnswer))
	a.sync(t)
	got.waitFor(t, "x"+cprAnswer)

	// Typing still takes the session.
	b.mw.Write([]byte("y"))
	b.sync(t)
	got.waitFor(t, "x"+cprAnswer+"y")
	if p := primary(); p != "b" {
		t.Errorf("after b typed the primary is %q, want b", p)
	}
}
