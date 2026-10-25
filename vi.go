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
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	pspkg "github.com/pborman/ps"
)

func PS(pid int) string {
	pm, err := pspkg.GetProcessMap()
	if err != nil {
		return err.Error()
	}
	p := pm.Pids[pid]
	if p == nil {
		return "process not found"
	}
	var buf bytes.Buffer
	printProc(&buf, p, "", nil)
	return buf.String()
}

func sanePath(p string) string {
	if strings.HasPrefix(p, user.HomeDir) {
		return "~" + p[len(user.HomeDir):]
	}
	return p
}

func processName(p *pspkg.Process) string {
	cmd, err := p.Command()
	if err == nil && cmd != "" {
		return path.Base(cmd)
	}
	argv, err := p.Argv()
	if err == nil && len(argv) > 0 {
		return path.Base(strings.TrimPrefix(argv[0], "-"))
	}
	return ""
}

func processWD(p *pspkg.Process) string {
	wd, err := p.Cwd()
	if err != nil || wd == "" {
		return "unknown"
	}
	return wd
}

func processFiles(p *pspkg.Process) []string {
	fds, err := p.Fds()
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(fds))
	for _, fd := range fds {
		if fd.Path != "" {
			files = append(files, fd.Path)
		}
	}
	return files
}

// printProc writes p and its descendants to w, one process per line.
// parent is the argv of p's parent, used to recognize forks.
func printProc(w io.Writer, p *pspkg.Process, prefix string, parent []string) {
	name := processName(p)
	wd := sanePath(processWD(p))
	argv, _ := p.Argv()
	switch name {
	case "pty":
		fmt.Fprintf(w, "%s pty %d (%s)\n", prefix, p.Pid(), wd)
	case "vi", "vi.exe", "vim", "nvim", "govi", "nvi":
		fmt.Fprintf(w, "%svi %s (%s)\n", prefix, viFiles(argv, processFiles(p)), wd)
	default:
		if len(argv) == 0 {
			fmt.Fprintf(w, "%s%s (%s)\n", prefix, name, wd)
		} else {
			fmt.Fprintf(w, "%s%s (%s)\n", prefix, argvLabel(argv, parent), wd)
		}
	}
	if prefix == "" {
		prefix = "\u2b11 "
	}
	for _, child := range p.Children {
		printProc(w, child, "  "+prefix, argv)
	}
}

// maxArgLen is the longest a single argument (or a summarized claude
// command) may be in a listing before it is shortened.
const maxArgLen = 64

// argvLabel returns a single line describing argv.  A process with the same
// argv as its parent is a fork (such as a shell subshell) and is not repeated.
// Commands run by Claude Code are reduced to the command Claude asked for.
// Control characters are escaped and long arguments are shortened so each
// process uses exactly one line.
func argvLabel(argv, parent []string) string {
	if parent != nil && slices.Equal(argv, parent) {
		if isShell(argv[0]) {
			return "(subshell)"
		}
		return "(fork)"
	}
	if s, ok := claudeCommand(argv); ok {
		return "claude$ " + s
	}
	args := make([]string, len(argv))
	for i, arg := range argv {
		args[i] = shorten(escapeControl(arg), maxArgLen)
	}
	return "[" + strings.Join(args, " ") + "]"
}

func isShell(arg0 string) bool {
	switch path.Base(strings.TrimPrefix(arg0, "-")) {
	case "sh", "ksh", "mksh", "bash", "zsh", "dash", "csh", "tcsh", "fish":
		return true
	}
	return false
}

// claudeCommand returns a summary of the command Claude Code is running if
// argv is Claude Code's shell wrapper.  The wrapper's argv is
//
//	argv[0]: zsh (or bash)
//	argv[1]: -c
//	argv[2]: source ~/.claude/shell-snapshots/X.sh ... && eval 'CMD' [< /dev/null] && pwd -P >| /tmp/claude-N-cwd
//
// where single quotes in CMD are written as '"'"'.
func claudeCommand(argv []string) (string, bool) {
	if len(argv) != 3 || argv[1] != "-c" || !strings.Contains(argv[2], "/.claude/shell-snapshots/") {
		return "", false
	}
	cmd, ok := claudeEval(argv[2])
	if !ok {
		return "(unrecognized command)", true
	}
	return summarizeScript(cmd), true
}

// claudeEval extracts CMD from the eval in Claude Code's wrapper and undoes
// its single quoting.
func claudeEval(s string) (string, bool) {
	const start = "eval '"
	i := strings.Index(s, start)
	if i < 0 {
		return "", false
	}
	s = s[i+len(start):]
	i = strings.LastIndex(s, " && pwd -P >| ")
	if i < 0 {
		return "", false
	}
	s = strings.TrimSuffix(s[:i], " < /dev/null")
	if !strings.HasSuffix(s, "'") {
		return "", false
	}
	return strings.ReplaceAll(s[:len(s)-1], `'"'"'`, "'"), true
}

var assignmentRE = regexp.MustCompile(`^(export\s+)?[A-Za-z_][A-Za-z0-9_]*=\S*$`)

// isAssignment reports if line only assigns variables, e.g., "A=1; B=$A/x".
func isAssignment(line string) bool {
	for _, part := range strings.Split(line, ";") {
		part = strings.TrimSpace(part)
		if part != "" && !assignmentRE.MatchString(part) {
			return false
		}
	}
	return true
}

// summarizeScript returns the first line of script that does more than
// assign variables, followed by a count of the other lines, if any.
// Blank lines and comments are ignored.
func summarizeScript(script string) string {
	var lines []string
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	show := lines[0]
	for _, line := range lines {
		if !isAssignment(line) {
			show = line
			break
		}
	}
	s := shorten(escapeControl(show), maxArgLen)
	switch n := len(lines) - 1; n {
	case 0:
	case 1:
		s += " (+1 line)"
	default:
		s += fmt.Sprintf(" (+%d lines)", n)
	}
	return s
}

// escapeControl returns s with control characters and invalid UTF-8 replaced
// by Go style escapes so s cannot span lines or affect the terminal.
func escapeControl(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case unicode.IsControl(r):
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

// shorten returns s if it has at most n runes, otherwise its first n-3 runes
// followed by "...".
func shorten(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-3]) + "..."
}

func viFiles(argv, files []string) []string {
	var out []string
	a := argv
	if len(a) > 0 {
		a = a[1:]
	}
	for len(a) > 0 && strings.HasPrefix(a[0], "-") {
		a = a[1:]
	}
	for _, file := range a {
		out = append(out, sanePath(file))
	}
	for _, file := range files {
		switch {
		case !strings.HasPrefix(file, "/"):
		case strings.HasPrefix(file, "/dev"):
		case strings.HasPrefix(file, "/private/dev"):
		case strings.HasPrefix(file, "/tmp/vi."):
		case strings.HasPrefix(file, "/private/tmp/vi."):
		case strings.HasPrefix(file, "/var/tmp/vi.recover"):
		case strings.HasPrefix(file, "/private/var/tmp/vi.recover"):
		case strings.Contains(file, "/.vim/swap"):
		default:
			out = append(out, sanePath(file))
		}
	}
	return out
}
