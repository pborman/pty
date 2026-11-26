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
	"strconv"
	"strings"
	"unicode/utf8"
)

// Besides the queries record.go knows about, config.yaml may name more
// sequences to keep out of the record, each optionally with the answer the
// terminal sends to it, which is then passed on from just one client (see
// answer.go):
//
//	sequences:
//	  - start: '\e]7717;ropen'
//	    answer:
//	      start: '#ROPEN '
//	      end: [' NEPOR#']
//	  - start: '\e_Gi=1,a=q'
//	    end: ['\e\\']
//	    limit: 65536
//
// A sequence (or answer) whose start begins with ESC [, ESC ] or ESC P is a
// CSI, OSC or DCS: it matches every one that begins with start, and ends
// where the terminal ends it, so it has no end or limit.  Any other start
// begins a sequence of its own, which runs until the earliest of its ends,
// or is just start if it has none.  If none of its ends is found within
// limit bytes (default 4096) it is taken not to be the sequence after all.
//
// The strings are written in single quotes, so that YAML leaves them alone,
// and pty decodes them itself:
//
//	\e        ESC
//	\a \b \f \n \r \t \v \\ \' \"
//	\NNN      the byte with octal value NNN
//	\xNN      the byte with hex value NN
//	\uNNNN    the character U+NNNN, in UTF-8
//	^X        the control character X: ^@ through ^_ (or ^a through ^z),
//	          and ^? for DEL
//	\^        ^
//
// YAML's own double-quoted escapes are not used because YAML reads "\004"
// as a NUL followed by "04", and "\x9b" as the character U+009B rather than
// the byte 0x9b.

// A SequenceConfig is one entry of the sequences list in config.yaml.
type SequenceConfig struct {
	Start  string        `yaml:"start"`
	End    []string      `yaml:"end"`
	Limit  int           `yaml:"limit"`
	Answer *AnswerConfig `yaml:"answer"`
}

// An AnswerConfig is the answer to a SequenceConfig.
type AnswerConfig struct {
	Start string   `yaml:"start"`
	End   []string `yaml:"end"`
	Limit int      `yaml:"limit"`
}

// builtinSequences are compiled before those in config.yaml.
var builtinSequences = []SequenceConfig{
	// ropen asks an iPadSSH terminal for its current receiver address,
	// and the terminal answers by typing "#ROPEN HOST:PORT/TOKEN NEPOR#".
	{Start: `\e]7717;ropen`, Answer: &AnswerConfig{Start: `#ROPEN `, End: []string{` NEPOR#`}}},
}

// defaultSequenceLimit is the default limit of a sequence of its own.
const defaultSequenceLimit = 4096

// A seqPattern is a compiled sequence or answer.
type seqPattern struct {
	start []byte
	kind  byte     // '[', ']' or 'P' for a CSI, OSC or DCS; 0 for one of its own
	end   []string // the ends of one of its own
	ends  *endSet  // end, compiled; nil if there are none
	limit int
}

// A seqRule is a compiled SequenceConfig.
type seqRule struct {
	seqPattern
	answer *seqPattern
	key    string // the answer's key (see answerKey), if there is an answer
}

// seqRules are the compiled builtinSequences and the sequences in
// config.yaml.  They are set when the configuration is read and are not
// changed after that.
var seqRules = mustCompileSequences(builtinSequences)

func mustCompileSequences(specs []SequenceConfig) []*seqRule {
	rules, err := compileSequences(specs)
	if err != nil {
		panic(err)
	}
	return rules
}

// compileSequences compiles specs.
func compileSequences(specs []SequenceConfig) ([]*seqRule, error) {
	var rules []*seqRule
	for i, spec := range specs {
		name := fmt.Sprintf("sequences[%d]", i)
		p, err := compilePattern(name, spec.Start, spec.End, spec.Limit)
		if err != nil {
			return nil, err
		}
		r := &seqRule{seqPattern: *p}
		if a := spec.Answer; a != nil {
			if r.answer, err = compilePattern(name+".answer", a.Start, a.End, a.Limit); err != nil {
				return nil, err
			}
			r.key = configAnswerKey(r.answer.start)
		}
		rules = append(rules, r)
	}
	if err := checkPrefixes(rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func compilePattern(name, start string, end []string, limit int) (*seqPattern, error) {
	if start == "" {
		return nil, fmt.Errorf("%s: no start", name)
	}
	s, err := decodeSequence(start)
	if err != nil {
		return nil, fmt.Errorf("%s.start: %v", name, err)
	}
	p := &seqPattern{start: []byte(s)}
	if len(s) >= 2 && s[0] == '\033' && strings.IndexByte("[]P", s[1]) >= 0 {
		if len(s) == 2 {
			return nil, fmt.Errorf("%s.start: %q would match every %s", name, start, map[byte]string{'[': "CSI", ']': "OSC", 'P': "DCS"}[s[1]])
		}
		if len(end) > 0 || limit != 0 {
			return nil, fmt.Errorf("%s: a start of %q is a CSI, OSC or DCS, which ends where the terminal ends it; remove end and limit", name, start)
		}
		p.kind = s[1]
		return p, nil
	}
	for j, e := range end {
		d, err := decodeSequence(e)
		if err != nil {
			return nil, fmt.Errorf("%s.end[%d]: %v", name, j, err)
		}
		if d == "" {
			return nil, fmt.Errorf("%s.end[%d]: empty", name, j)
		}
		p.end = append(p.end, d)
	}
	switch {
	case limit < 0:
		return nil, fmt.Errorf("%s.limit: %d is negative", name, limit)
	case limit == 0:
		limit = defaultSequenceLimit
	}
	p.limit = limit
	p.ends = newEndSet(p.end, "")
	return p, nil
}

// builtinPrefixes are the sequences registered by addRecordFilters that a
// sequence of its own must not overlap.
var builtinPrefixes = []string{csiIntro, oscIntro, dcsIntro, decid, enq}

// checkPrefixes reports an error if the start of a sequence of its own in
// rules is a prefix of another (or of a built-in one), since the shorter
// would always be found first.
func checkPrefixes(rules []*seqRule) error {
	var starts []string
	for _, r := range rules {
		if r.kind == 0 {
			starts = append(starts, string(r.start))
		}
	}
	for i, s := range starts {
		for _, b := range builtinPrefixes {
			if strings.HasPrefix(s, b) || strings.HasPrefix(b, s) {
				return fmt.Errorf("sequence start %q overlaps pty's own %q", s, b)
			}
		}
		for _, o := range starts[i+1:] {
			if strings.HasPrefix(s, o) || strings.HasPrefix(o, s) {
				return fmt.Errorf("sequence starts %q and %q overlap", s, o)
			}
		}
	}
	return nil
}

// configAnswerKey returns the key of an answer from config.yaml that starts
// with start.
func configAnswerKey(start []byte) string {
	return "answer " + strconv.Quote(string(start))
}

// decodeSequence decodes the escapes and caret notation in s (see above).
func decodeSequence(s string) (string, error) {
	if strings.IndexByte(s, 0) >= 0 {
		return "", fmt.Errorf("%q contains a NUL; write the string in single quotes ('...') so YAML does not read \\0 as a NUL", s)
	}
	var out []byte
	in := s
	for len(in) > 0 {
		c := in[0]
		switch {
		case c == '^':
			if len(in) < 2 {
				return "", fmt.Errorf("%q ends with ^ (write \\^ for a ^)", s)
			}
			switch x := in[1]; {
			case x == '?':
				out = append(out, 0x7f)
			case x >= '@' && x <= '_':
				out = append(out, x-'@')
			case x >= 'a' && x <= 'z':
				out = append(out, x-'a'+1)
			default:
				return "", fmt.Errorf("%q: ^%c is not a control character (write \\^ for a ^)", s, x)
			}
			in = in[2:]
		case c != '\\':
			out = append(out, c)
			in = in[1:]
		case len(in) < 2:
			return "", fmt.Errorf("%q ends with \\", s)
		case in[1] == 'e':
			out = append(out, '\033')
			in = in[2:]
		case in[1] == '^' || in[1] == '\'' || in[1] == '"':
			out = append(out, in[1])
			in = in[2:]
		default:
			v, multibyte, tail, err := strconv.UnquoteChar(in, 0)
			if err != nil {
				return "", fmt.Errorf("%q: bad escape at %q", s, in)
			}
			if multibyte {
				out = utf8.AppendRune(out, v)
			} else {
				out = append(out, byte(v))
			}
			in = tail
		}
	}
	if bytes.IndexByte(out, 0) >= 0 {
		return "", fmt.Errorf("%q: NUL is not allowed", s)
	}
	return string(out), nil
}

// tooLong reports whether raw, a complete sequence that starts with p.start
// and ends with one of p.end, has more than p.limit bytes between them.
func (p *seqPattern) tooLong(raw []byte) bool {
	n := len(raw) - len(p.start)
	for _, e := range p.end {
		if bytes.HasSuffix(raw, []byte(e)) {
			n -= len(e)
			break
		}
	}
	return n > p.limit
}

// matchRule returns the first rule for a sequence of kind ('[', ']' or 'P')
// that raw begins with, or nil.
func matchRule(kind byte, raw []byte) *seqRule {
	for _, r := range seqRules {
		if r.kind == kind && bytes.HasPrefix(raw, r.start) {
			return r
		}
	}
	return nil
}
