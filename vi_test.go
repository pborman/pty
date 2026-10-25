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
	"os"
	"strings"
	"testing"
)

func TestSanePath(t *testing.T) {
	home := user.HomeDir
	if got := sanePath(home + "/src/pty"); got != "~/src/pty" {
		t.Errorf("sanePath(home/...) = %q", got)
	}
	if got := sanePath("/tmp/x"); got != "/tmp/x" {
		t.Errorf("sanePath(/tmp/x) = %q", got)
	}
}

func TestViFiles(t *testing.T) {
	home := user.HomeDir
	argv := []string{"vi", "-n", home + "/file.go", "other"}
	files := []string{
		"/dev/tty",
		"/private/dev/null",
		"/tmp/vi.foo",
		"/private/tmp/vi.bar",
		"/var/tmp/vi.recover/x",
		"/private/var/tmp/vi.recover/y",
		home + "/.vim/swap/file.go.swp",
		"relative",
		"/etc/passwd",
		home + "/notes",
	}
	got := viFiles(argv, files)
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "file.go") || !strings.Contains(joined, "other") {
		t.Errorf("argv files missing: %q", got)
	}
	if !strings.Contains(joined, "/etc/passwd") {
		t.Errorf("regular file missing: %q", got)
	}
	if !strings.Contains(joined, "notes") {
		t.Errorf("home file missing: %q", got)
	}
	for _, skip := range []string{"/dev/tty", "/private/dev", "/tmp/vi.", "/private/tmp/vi.", "vi.recover", ".vim/swap", "relative"} {
		if strings.Contains(joined, skip) {
			t.Errorf("filtered path %q present in %q", skip, got)
		}
	}
}

func TestPSMissing(t *testing.T) {
	got := PS(-1)
	if got == "" {
		t.Error("PS(-1) returned empty")
	}
	if got != "process not found" && !strings.Contains(got, "process not found") {
		t.Log(got)
	}
}

func TestPSSelf(t *testing.T) {
	got := PS(os.Getpid())
	if got == "process not found" || got == "" {
		t.Fatalf("PS(self) = %q", got)
	}
	t.Log(got)
}

// wrapClaude returns the argv Claude Code uses to run cmd in a shell.
// If devNull is set the eval is followed by "< /dev/null".
func wrapClaude(cmd string, devNull bool) []string {
	redirect := ""
	if devNull {
		redirect = " < /dev/null"
	}
	return []string{
		"/bin/zsh",
		"-c",
		"source /dir1/.claude/shell-snapshots/file1.sh 2>/dev/null || true" +
			" && setopt NO_EXTENDED_GLOB NO_BARE_GLOB_QUAL 2>/dev/null || true" +
			` && { \builtin unalias -- 'cmd1'; \builtin unset -f -- 'cmd1'; } >/dev/null 2>&1 || true` +
			" && eval '" + strings.ReplaceAll(cmd, "'", `'"'"'`) + "'" + redirect +
			" && pwd -P >| /dir2/file2",
	}
}

func TestEscapeControl(t *testing.T) {
	for _, tt := range []struct {
		in, out string
	}{
		{"", ""},
		{"cmd1 file1", "cmd1 file1"},
		{"a\nb", `a\nb`},
		{"a\r\tb", `a\r\tb`},
		{"\x1b[0m", `\x1b[0m`},
		{"\x7f", `\x7f`},
		{"\u0085", `\x85`},
		{"\xff", `\xff`},
		{"diré1", "diré1"},
	} {
		if got := escapeControl(tt.in); got != tt.out {
			t.Errorf("escapeControl(%q) = %q, want %q", tt.in, got, tt.out)
		}
	}
}

func TestShorten(t *testing.T) {
	for _, tt := range []struct {
		in  string
		n   int
		out string
	}{
		{"file1", 5, "file1"},
		{"file1", 10, "file1"},
		{"file12", 5, "fi..."},
		{"éééééé", 5, "éé..."},
	} {
		if got := shorten(tt.in, tt.n); got != tt.out {
			t.Errorf("shorten(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.out)
		}
	}
}

func TestIsShell(t *testing.T) {
	for _, tt := range []struct {
		in    string
		shell bool
	}{
		{"-ksh", true},
		{"/bin/zsh", true},
		{"bash", true},
		{"cmd1", false},
		{"/dir1/cmd1", false},
	} {
		if got := isShell(tt.in); got != tt.shell {
			t.Errorf("isShell(%q) = %v, want %v", tt.in, got, tt.shell)
		}
	}
}

func TestIsAssignment(t *testing.T) {
	for _, tt := range []struct {
		in     string
		assign bool
	}{
		{"V1=/dir1/file1", true},
		{"V1=$V2/dir1; V3=$V1/file1.txt", true},
		{"export V1=/dir1", true},
		{"V1=", true},
		{"V1=x cmd1", false},
		{"cmd1 V1=x", false},
		{"V1=x; cmd1", false},
		{"1V=x", false},
		{"cmd1", false},
	} {
		if got := isAssignment(tt.in); got != tt.assign {
			t.Errorf("isAssignment(%q) = %v, want %v", tt.in, got, tt.assign)
		}
	}
}

func TestClaudeEval(t *testing.T) {
	cmd := "V1=/dir1/file1\ncmd1 -n 0 -f $V1 | cmd2 'x == \"y\" { print }'\ncmd3 '+%Y'"
	for _, devNull := range []bool{true, false} {
		argv := wrapClaude(cmd, devNull)
		got, ok := claudeEval(argv[2])
		if !ok {
			t.Errorf("devNull=%v: claudeEval failed", devNull)
			continue
		}
		if got != cmd {
			t.Errorf("devNull=%v: claudeEval got\n%s\nwant\n%s", devNull, got, cmd)
		}
	}

	for _, s := range []string{
		"source /dir1/.claude/shell-snapshots/file1.sh && cmd1",
		"source /dir1/.claude/shell-snapshots/file1.sh && eval 'cmd1'",
		"source /dir1/.claude/shell-snapshots/file1.sh && eval 'cmd1 && pwd -P >| /dir2/file2",
	} {
		if got, ok := claudeEval(s); ok {
			t.Errorf("claudeEval(%q) = %q, want failure", s, got)
		}
	}
}

func TestSummarizeScript(t *testing.T) {
	long := "cmd1 " + strings.Repeat("x", 100)
	for _, tt := range []struct {
		name, in, out string
	}{
		{"empty", "", ""},
		{"one line", "cmd1 -v file1", "cmd1 -v file1"},
		{"two lines", "cmd1 file1\ncmd2", "cmd1 file1 (+1 line)"},
		{
			"skip assignments",
			"V1=/dir1/file1\nV2=$V1/dir2; V3=$V2/file2\n( cmd1 -f $V1 | cmd2 -v v=$V3 '\n  /x/ { print }\n' ) &\nw=$!\nwhile cmd3 -0 $w; do\n  cmd4 30\ndone",
			"( cmd1 -f $V1 | cmd2 -v v=$V3 ' (+8 lines)",
		},
		{"only assignments", "V1=x\nV2=y", "V1=x (+1 line)"},
		{"blank and comments", "\n# cmd9\n\n  cmd1 file1  \n\n", "cmd1 file1"},
		{"long line", long, long[:maxArgLen-3] + "..."},
		{"control", "cmd1 \x1b[0m", `cmd1 \x1b[0m`},
	} {
		if got := summarizeScript(tt.in); got != tt.out {
			t.Errorf("%s: summarizeScript got %q, want %q", tt.name, got, tt.out)
		}
	}
}

func TestArgvLabel(t *testing.T) {
	script := "V1=/dir1/file1\n( cmd1 -f $V1 | cmd2 'x' ) &\ncmd3 30"
	claude := wrapClaude(script, true)
	long := "/dir1/" + strings.Repeat("dir2/", 20) + "file1"

	for _, tt := range []struct {
		name         string
		argv, parent []string
		out          string
	}{
		{"plain", []string{"cmd1", "-n", "file1"}, nil, "[cmd1 -n file1]"},
		{"login shell", []string{"-ksh"}, []string{"cmd1"}, "[-ksh]"},
		{"newlines", []string{"cmd1", "-e", "x\ny"}, nil, `[cmd1 -e x\ny]`},
		{"long arg", []string{"cmd1", long}, nil, "[cmd1 " + long[:maxArgLen-3] + "...]"},
		{"claude", claude, []string{"cmd9"}, "claude$ ( cmd1 -f $V1 | cmd2 'x' ) & (+2 lines)"},
		{"claude no parent", claude, nil, "claude$ ( cmd1 -f $V1 | cmd2 'x' ) & (+2 lines)"},
		{"claude no devnull", wrapClaude("cmd1 file1", false), nil, "claude$ cmd1 file1"},
		{"claude subshell", claude, claude, "(subshell)"},
		{"shell subshell", []string{"-ksh"}, []string{"-ksh"}, "(subshell)"},
		{"fork", []string{"cmd1", "file1"}, []string{"cmd1", "file1"}, "(fork)"},
		{"not a fork", []string{"cmd1", "file1"}, []string{"cmd1", "file2"}, "[cmd1 file1]"},
		{
			"unparsable claude",
			[]string{"/bin/zsh", "-c", "source /dir1/.claude/shell-snapshots/file1.sh && cmd1"},
			nil,
			"claude$ (unrecognized command)",
		},
		{
			"plain shell -c",
			[]string{"/bin/zsh", "-c", "cmd1 file1\ncmd2"},
			nil,
			`[/bin/zsh -c cmd1 file1\ncmd2]`,
		},
		{
			"claude extra args",
			append(wrapClaude("cmd1", true), "file1"),
			nil,
			"[/bin/zsh -c " + wrapClaude("cmd1", true)[2][:maxArgLen-3] + "... file1]",
		},
	} {
		got := argvLabel(tt.argv, tt.parent)
		if got != tt.out {
			t.Errorf("%s: argvLabel got\n%s\nwant\n%s", tt.name, got, tt.out)
		}
		if strings.ContainsAny(got, "\n\r") {
			t.Errorf("%s: argvLabel contains a newline: %q", tt.name, got)
		}
	}
}
