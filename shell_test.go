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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain selects the login shell as main does, since tests such as
// TestNewShellAndSetenv need one.
func TestMain(m *testing.M) {
	SelectLoginShell(nil)
	os.Exit(m.Run())
}

func TestSelectLoginShell(t *testing.T) {
	dir := t.TempDir()
	shell := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cfg1, cfg2, env, base1, base2 := shell("cfg1"), shell("cfg2"), shell("env"), shell("base1"), shell("base2")
	missing := filepath.Join(dir, "missing")
	notExec := filepath.Join(dir, "notexec")
	if err := os.WriteFile(notExec, []byte("#!/bin/sh\n"), 0644); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	// Bare names are looked up in $PATH.
	t.Setenv("PATH", dir)

	oldBase, oldLogin := baseShells, LoginShell
	defer func() { baseShells, LoginShell = oldBase, oldLogin }()

	for _, tt := range []struct {
		name   string
		config []string
		base   []string // baseShells: $SHELL, then the built-in list
		want   string
	}{
		{"config first", []string{cfg1, cfg2}, []string{env, base1}, cfg1},
		{"missing config skipped", []string{missing, cfg2}, []string{env, base1}, cfg2},
		{"no config uses $SHELL", nil, []string{env, base1}, env},
		{"no config found uses $SHELL", []string{missing}, []string{env, base1}, env},
		{"empty entries skipped", []string{""}, []string{"", base1}, base1},
		{"$SHELL missing", nil, []string{missing, base1, base2}, base1},
		{"built-in list in order", nil, []string{"", missing, base2, base1}, base2},
		{"none", []string{missing}, []string{"", missing}, ""},
		{"directory skipped", []string{subdir, cfg2}, []string{env}, cfg2},
		{"not executable skipped", []string{notExec}, []string{env}, env},
		{"name found in $PATH", []string{"cfg2"}, []string{env}, cfg2},
		{"name not in $PATH", []string{"missing", "notexec"}, []string{"base1"}, base1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			baseShells = tt.base
			LoginShell = ""
			SelectLoginShell(tt.config)
			if LoginShell != tt.want {
				t.Errorf("LoginShell = %q, want %q", LoginShell, tt.want)
			}
		})
	}
}

func TestShellEnv(t *testing.T) {
	got := shellEnv([]string{"A=1", "SHELL=/bin/ksh", "B=2", "SHELLX=y"}, "/bin/zsh")
	want := []string{"SHELL=/bin/zsh", "A=1", "B=2", "SHELLX=y"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("shellEnv = %q, want %q", got, want)
	}
	if got := shellEnv(nil, "/bin/sh"); fmt.Sprint(got) != "[SHELL=/bin/sh]" {
		t.Errorf("shellEnv(nil) = %q", got)
	}
}

// TestBaseShells checks that $SHELL comes before the built-in shells.
func TestBaseShells(t *testing.T) {
	if baseShells[0] != os.Getenv("SHELL") {
		t.Errorf("baseShells[0] = %q, want $SHELL (%q)", baseShells[0], os.Getenv("SHELL"))
	}
	for _, sh := range baseShells[1:] {
		if !filepath.IsAbs(sh) {
			t.Errorf("built-in shell %q is not an absolute path", sh)
		}
	}
}

// TestReadConfigShells checks that the shells list is read from config.yaml.
func TestReadConfigShells(t *testing.T) {
	defer withTempHome(t)()
	oldConfig, oldRules := config, seqRules
	defer func() { config, seqRules = oldConfig, oldRules }()
	dir := filepath.Join(user.HomeDir, rcdir)
	os.MkdirAll(dir, 0700)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("shells:\n  - /opt/bin/zsh\n  - /bin/zsh\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ReadConfig(); err != nil {
		t.Fatal(err)
	}
	if len(config.Shells) != 2 || config.Shells[0] != "/opt/bin/zsh" || config.Shells[1] != "/bin/zsh" {
		t.Errorf("config.Shells = %q", config.Shells)
	}
}

func TestNewShellAndSetenv(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("shellsess", "")
	defer sess.Remove()

	s := NewShell(sess)
	if s.Shell == "" {
		t.Fatal("LoginShell was empty")
	}
	if s.session != sess {
		t.Error("session not attached")
	}
	if s.Done() {
		t.Error("new shell should not be Done")
	}

	s.Setenv("FOO", "bar")
	found := false
	for _, e := range s.Env {
		if e == "FOO=bar" {
			found = true
		}
	}
	if !found {
		t.Errorf("Setenv did not add FOO=bar: %q", s.Env)
	}
	s.Setenv("FOO", "baz")
	count := 0
	for _, e := range s.Env {
		if len(e) >= 4 && e[:4] == "FOO=" {
			count++
			if e != "FOO=baz" {
				t.Errorf("replaced value = %q", e)
			}
		}
	}
	if count != 1 {
		t.Errorf("FOO appears %d times", count)
	}
}

func TestShellAttachDetachListTake(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("shellclients", "")
	defer sess.Remove()
	s := NewShell(sess)

	var buf bytes.Buffer
	c1 := NewClient(&buf)
	c1.SetName("one")
	c1.SetPid(os.Getpid())
	n := s.Attach(c1)
	if n != 0 {
		t.Errorf("first Attach index = %d", n)
	}
	if s.CountClients() != 1 {
		t.Errorf("CountClients = %d, want 1", s.CountClients())
	}

	c2 := NewClient(&bytes.Buffer{})
	c2.SetName("two")
	s.Attach(c2)
	if s.CountClients() != 1 {
		// c2 has no pid so it is not "active"
		t.Logf("CountClients with inactive client = %d", s.CountClients())
	}

	s.Take(c1, false)
	s.Take(c1, false) // already primary
	s.Take(c2, false)

	s.List(c1)
	s.AddPid(c1, os.Getpid())
	if s.Count() != 1 {
		t.Errorf("Count = %d, want 1", s.Count())
	}
	s.AddPid(c2, -1)
	s.Count() // should drop the dead pid

	s.Detach(c2)
	s.Detach(c1)
	stopClient(c1)
	stopClient(c2)
}

func TestShellStartAlreadyStarted(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("started", "")
	defer sess.Remove()
	s := NewShell(sess)
	s.Shell = "/bin/example-sh"
	s.Setenv("SHELL", "/bin/client-sh")
	s.pty = os.Stdin
	if err := s.Start(false); err == nil {
		t.Error("Start on already-started shell succeeded")
	}
	// Start sets $SHELL to the shell the session runs.
	n := 0
	for _, e := range s.Env {
		if len(e) > 6 && e[:6] == "SHELL=" {
			n++
			if e != "SHELL=/bin/example-sh" {
				t.Errorf("Env has %q, want SHELL=/bin/example-sh", e)
			}
		}
	}
	if n != 1 {
		t.Errorf("SHELL is in Env %d times", n)
	}
}

func TestShellDropsRopenQuery(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("ropensess", "")
	defer sess.Remove()

	s := NewShell(sess)
	s.eb.Write([]byte("before" + ropenQuery + "after"))
	// The query split across writes must be dropped too.
	s.eb.Write([]byte("x" + ropenQuery[:4]))
	s.eb.Write([]byte(ropenQuery[4:] + "y"))
	if got, want := string(s.eb.normal), "beforeafterxy"; got != want {
		t.Errorf("recorded %q, want %q", got, want)
	}
}

func TestShellDropsOSC52(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("osc52sess", "")
	defer sess.Remove()

	for _, tt := range []struct {
		name   string
		writes []string
		want   string
	}{
		{"BEL", []string{"a\033]52;c;aGk=\007b"}, "ab"},
		{"ST", []string{"a\033]52;c;aGk=\033\\b"}, "ab"},
		{"query", []string{"a\033]52;c;?\007b"}, "ab"},
		{"two in one write", []string{"a\033]52;c;eA==\007b\033]52;p;eQ==\033\\c"}, "abc"},
		{"earliest terminator wins", []string{"a\033]52;c;eA==\007b\033\\c"}, "ab\033\\c"},
		{"split intro", []string{"a\033]5", "2;c;aGk=\007b"}, "ab"},
		{"split body", []string{"a\033]52;c;aG", "k=\007b"}, "ab"},
		{"split before ST", []string{"a\033]52;c;aGk=", "\033\\b"}, "ab"},
		{"split inside ST", []string{"a\033]52;c;aGk=\033", "\\b"}, "ab"},
		{"split inside ST, then more", []string{"a\033]52;c;aGk=\033", "\\b\033]52;c;?\007c"}, "abc"},
		// ESC ends the OSC; the ESC and the BEL after it are output.
		{"ESC alone at end, BEL next", []string{"a\033]52;c;aGk=\033", "\007b"}, "a\033\007b"},
		{"ended by CSI", []string{"a\033]52;c;aGk=\033[31mb"}, "a\033[31mb"},
		{"ended by CSI, split after ESC", []string{"a\033]52;c;aGk=\033", "[31mb"}, "a\033[31mb"},
		{"ended by another OSC", []string{"a\033]52;c;aGk=\033]0;t\007b"}, "a\033]0;t\007b"},
		{"ended by another OSC 52", []string{"a\033]52;c;aGk=\033]52;c;?\007b"}, "ab"},
		{"CAN", []string{"a\033]52;c;aGk=\030b"}, "ab"},
		{"SUB", []string{"a\033]52;c;aGk=\032b"}, "ab"},
		{"byte at a time, ended by CSI", split1("a\033]52;c;aGk=\033[31mb"), "a\033[31mb"},
		{"byte at a time, ST", split1("a\033]52;c;aGk=\033\\b"), "ab"},
		{"byte at a time, BEL", split1("a\033]52;c;?\007b"), "ab"},
		{"other OSC kept", []string{"a\033]0;title\007b"}, "a\033]0;title\007b"},
		{"OSC 520 kept", []string{"a\033]520;x\007b"}, "a\033]520;x\007b"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := NewShell(sess)
			for _, w := range tt.writes {
				s.eb.Write([]byte(w))
			}
			s.eb.Flush()
			if got := string(s.eb.normal); got != tt.want {
				t.Errorf("recorded %q, want %q", got, tt.want)
			}
		})
	}
}

// split1 returns s as a list of one-byte strings.
func split1(s string) []string {
	out := make([]string, len(s))
	for i := range s {
		out[i] = s[i : i+1]
	}
	return out
}

func TestShellWriteCtrlCBreaksSequence(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("ctrlcsess", "")
	defer sess.Remove()

	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()

	s := NewShell(sess)
	s.pty = devnull
	s.eb.Write([]byte("a\033]52;c;never ending"))
	s.eb.Write([]byte("lost"))
	if _, err := s.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	s.eb.Write([]byte("lost too"))
	if _, err := s.Write([]byte("x\003y")); err != nil {
		t.Fatal(err)
	}
	s.eb.Write([]byte("b"))
	s.eb.Flush()
	if got, want := string(s.eb.normal), "ab"; got != want {
		t.Errorf("recorded %q, want %q", got, want)
	}
}

// TestExecsh runs execsh in a child process, with LoginShell set to a script
// that prints its name and $SHELL.
func TestExecsh(t *testing.T) {
	if sh := os.Getenv("PTY_TEST_EXECSH"); sh != "" {
		LoginShell = sh
		execsh()
		return
	}
	script := filepath.Join(t.TempDir(), "mysh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$0 $SHELL\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestExecsh$")
	cmd.Env = append(os.Environ(), "PTY_TEST_EXECSH="+script, "SHELL=/bin/client-sh")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	// /bin/sh sets $0 to the script's path, not to argv[0].
	if got, want := strings.TrimSpace(string(out)), script+" "+script; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
