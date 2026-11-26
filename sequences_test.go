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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	yaml "gopkg.in/yaml.v2"
)

func TestDecodeSequence(t *testing.T) {
	for _, tt := range []struct {
		in, want string
	}{
		{`abc`, "abc"},
		{`\e]52;`, "\033]52;"},
		{`\a\b\f\n\r\t\v`, "\a\b\f\n\r\t\v"},
		{`\\ \' \"`, `\ ' "`},
		{`\004`, "\004"},
		{`\x04\x9b`, "\x04\x9b"},
		{`\u009b`, "\u009b"},
		{`^D^[^\^]^^^_^?`, "\004\033\034\035\036\037\177"},
		{`^d^a^z`, "\004\001\032"},
		{`\^D`, "^D"},
		{`é☺`, "é☺"},
	} {
		got, err := decodeSequence(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("decodeSequence(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
	for _, tt := range []struct {
		in, err string
	}{
		{"\x00", "single quotes"},
		{"\x0004", "single quotes"}, // what YAML makes of "\004"
		{`^@`, "NUL is not allowed"},
		{`\000`, "NUL is not allowed"},
		{`^`, `ends with ^`},
		{`a^1`, `^1 is not a control character`},
		{`\`, `ends with \`},
		{`\q`, `bad escape`},
		{`\400`, `bad escape`},
	} {
		if _, err := decodeSequence(tt.in); err == nil || !strings.Contains(err.Error(), tt.err) {
			t.Errorf("decodeSequence(%q) error = %v, want one containing %q", tt.in, err, tt.err)
		}
	}
}

// parseSequences parses a sequences list as it would appear in config.yaml.
func parseSequences(t *testing.T, y string) []SequenceConfig {
	t.Helper()
	var c struct{ Sequences []SequenceConfig }
	if err := yaml.Unmarshal([]byte("sequences:\n"+y), &c); err != nil {
		t.Fatal(err)
	}
	return c.Sequences
}

func TestCompileSequencesErrors(t *testing.T) {
	for _, tt := range []struct {
		yaml, err string
	}{
		{"- end: ['x']", "sequences[0]: no start"},
		{"- start: '\\e]1;'\n  end: ['\\a']", "remove end and limit"},
		{"- start: '\\e[?9'\n  limit: 10", "remove end and limit"},
		{"- start: '\\e]'", "would match every OSC"},
		{"- start: '\\e['", "would match every CSI"},
		{"- start: '\\eP'", "would match every DCS"},
		{"- start: '\\e_'\n  limit: -1", "sequences[0].limit: -1 is negative"},
		{"- start: '\\e_'\n  end: ['']", "sequences[0].end[0]: empty"},
		{"- start: '\\e_'\n  end: ['\\q']", "sequences[0].end[0]: "},
		{"- start: '\\q'", "sequences[0].start: "},
		{"- start: \"\\004x\"", "single quotes"},
		{"- start: '\\eZ1'", `overlaps pty's own "\x1bZ"`},
		{"- start: '\\e'", `overlaps pty's own "\x1b["`},
		{"- start: '^Ex'", `overlaps pty's own "\x05"`},
		{"- start: '\\e_A'\n- start: '\\e_AB'", "overlap"},
		{"- start: '\\e_A'\n  answer:\n    end: ['x']", "sequences[0].answer: no start"},
		{"- start: '\\e_A'\n  answer:\n    start: '\\e]7;'\n    end: ['\\a']", "sequences[0].answer: a start"},
	} {
		_, err := compileSequences(parseSequences(t, tt.yaml))
		if err == nil || !strings.Contains(err.Error(), tt.err) {
			t.Errorf("%s:\n  error = %v, want one containing %q", tt.yaml, err, tt.err)
		}
	}
}

func TestCompileSequences(t *testing.T) {
	rules, err := compileSequences(parseSequences(t, `
- start: '\e]1337;Custom'
- start: '\e_Gi=1,a=q'
  end: ['\e\\', '^G']
  limit: 100
  answer:
    start: '\e_Gi=1;'
    end: ['\e\\']
- start: '^F^Gx'
  answer:
    start: '#X '
    end: [' X#']
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 {
		t.Fatalf("got %d rules", len(rules))
	}
	if r := rules[0]; r.kind != ']' || string(r.start) != "\033]1337;Custom" || r.answer != nil || r.key != "" {
		t.Errorf("rule 0 = %+v", r)
	}
	if r := rules[1]; r.kind != 0 || r.limit != 100 || strings.Join(r.end, ",") != "\033\\,\a" || r.answer.kind != 0 || r.key != `answer "\x1b_Gi=1;"` {
		t.Errorf("rule 1 = %+v", r)
	}
	if r := rules[2]; r.limit != defaultSequenceLimit || r.ends != nil || r.answer.limit != defaultSequenceLimit {
		t.Errorf("rule 2 = %+v", r)
	}
}

func TestMustCompileSequencesPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	mustCompileSequences([]SequenceConfig{{}})
}

// withSequences sets seqRules to the built-in sequences and those in y for
// the rest of the test.
func withSequences(t *testing.T, y string) {
	t.Helper()
	rules, err := compileSequences(append(append([]SequenceConfig{}, builtinSequences...), parseSequences(t, y)...))
	if err != nil {
		t.Fatal(err)
	}
	old := seqRules
	seqRules = rules
	t.Cleanup(func() { seqRules = old })
}

func TestReadConfigSequences(t *testing.T) {
	defer withTempHome(t)()
	oldConfig, oldRules := config, seqRules
	defer func() { config, seqRules = oldConfig, oldRules }()
	dir := filepath.Join(user.HomeDir, rcdir)
	os.MkdirAll(dir, 0700)
	path := filepath.Join(dir, "config.yaml")
	if err := ioutil.WriteFile(path, []byte("sequences:\n  - start: '\\e_X'\n    end: ['^G']\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ReadConfig(); err != nil {
		t.Fatal(err)
	}
	if len(seqRules) != len(builtinSequences)+1 || string(seqRules[len(seqRules)-1].start) != "\033_X" {
		t.Errorf("seqRules = %+v", seqRules)
	}
	if err := ioutil.WriteFile(path, []byte("sequences:\n  - start: '\\eZ'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ReadConfig(); err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Errorf("bad sequence: error = %v", err)
	}
}

func TestConfiguredSequencesRecord(t *testing.T) {
	withSequences(t, `
- start: '\e_Gi=1,a=q'
  end: ['\e\\', '^G']
  limit: 20
- start: '^Fhello'
- start: '\e]1337;Custom'
- start: '\e[?5555'
- start: '\eP=9'
`)
	for _, tt := range []struct {
		name, in, want string
	}{
		{"own, ST", "a\033_Gi=1,a=q;xyz\033\\b", "ab"},
		{"own, BEL", "a\033_Gi=1,a=q\ab", "ab"},
		{"own, earliest end", "a\033_Gi=1,a=q\a\033\\b", "a\033\\b"},
		{"own, too long", "a\033_Gi=1,a=q" + strings.Repeat("x", 30) + "\ab", "a\033_Gi=1,a=q" + strings.Repeat("x", 30) + "\ab"},
		{"own, at the limit", "a\033_Gi=1,a=q" + strings.Repeat("x", 20) + "\ab", "ab"},
		{"own, no end", "a\006hellob", "ab"},
		{"own, not quite", "a\006helpb", "a\006helpb"},
		{"OSC", "a\033]1337;CustomThing=1\007b", "ab"},
		{"OSC, other", "a\033]1337;SetMark\007b", "a\033]1337;SetMark\007b"},
		{"CSI", "a\033[?5555;1nb", "ab"},
		{"CSI, other", "a\033[?555hb", "a\033[?555hb"},
		{"DCS", "a\033P=9;1q\033\\b", "ab"},
		{"DCS, other", "a\033P=8q\033\\b", "a\033P=8q\033\\b"},
	} {
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
	// A long configured OSC or DCS is dropped as it arrives.
	long := strings.Repeat("x", stringLimit)
	for _, in := range []string{"a\033]1337;Custom" + long + "\007b", "a\033P=9" + long + "\033\\b"} {
		if got, _ := recordOf(in, false); got != "ab" {
			t.Errorf("long %q...: recorded %d bytes", in[:10], len(got))
		}
	}
}

func TestConfiguredAnswers(t *testing.T) {
	withSequences(t, `
- start: '\e_Gi=1,a=q'
  end: ['\e\\']
  answer:
    start: '\e_Gi=1;'
    end: ['\e\\']
- start: '\e]1337;Custom'
  answer:
    start: '\e]1337;Custom='
- start: '\e[?5555n'
  answer:
    start: '^Fok'
`)
	for _, tt := range []struct {
		name, query, answer string
	}{
		{"own", "\033_Gi=1,a=q\033\\", "\033_Gi=1;OK\033\\"},
		{"OSC", "\033]1337;Custom\007", "\033]1337;Custom=42\007"},
		{"exact", "\033[?5555n", "\006ok"},
		{"ropen", ropenQuery, "#ROPEN 10.0.0.1:1234/abcd NEPOR#"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newAnswerShell(t)
			s.eb.Write([]byte(tt.query))
			if len(s.waits) != 1 {
				t.Fatalf("expected %q", waitKeys(s))
			}
			decide := func(key string) (bool, bool) { return s.waits[key] != nil, false }
			// Split into two reads at each place, except just
			// after a leading ESC, which is passed on at once as
			// the escape key.
			for i := 0; i < len(tt.answer); i++ {
				if i == 1 && tt.answer[0] == '\033' {
					continue
				}
				var f answerFilter
				out, typed := f.Filter([]byte(tt.answer[:i]), decide)
				o, ty := f.Filter([]byte(tt.answer[i:]), decide)
				out, typed = append(append([]byte{}, out...), o...), typed || ty
				if len(out) != 0 || typed {
					t.Errorf("split at %d: answer not dropped: %q %v", i, out, typed)
				}
			}
		})
	}
	// Without the query, the answers are just input.
	var f answerFilter
	in := "\033_Gi=1;OK\033\\\033]1337;Custom=42\007\006ok"
	out, typed := f.Filter([]byte(in), func(string) (bool, bool) { return false, false })
	if string(out) != in || !typed {
		t.Errorf("unexpected answers: got %q %v", out, typed)
	}
}
