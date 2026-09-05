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
	"os"
	"strings"
	"testing"

	"github.com/pborman/pty/proc"
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

func TestViFilesAndPrintProc(t *testing.T) {
	home := user.HomeDir
	p := &proc.Process{
		Pid:  42,
		Name: "vi",
		Argv: []string{"vi", "-n", home + "/file.go", "other"},
		WD:   home + "/src",
		Files: []string{
			"/dev/tty",
			"/tmp/vi.foo",
			"/var/tmp/vi.recover/x",
			"relative",
			"/etc/passwd",
			home + "/notes",
		},
	}
	files := viFiles(p)
	joined := strings.Join(files, ",")
	if !strings.Contains(joined, "file.go") || !strings.Contains(joined, "other") {
		t.Errorf("argv files missing: %q", files)
	}
	if !strings.Contains(joined, "/etc/passwd") {
		t.Errorf("regular file missing: %q", files)
	}
	if strings.Contains(joined, "/dev/tty") || strings.Contains(joined, "/tmp/vi.") {
		t.Errorf("filtered files present: %q", files)
	}

	var buf bytes.Buffer
	printProc(&buf, p, "")
	if !strings.Contains(buf.String(), "vi") {
		t.Errorf("print vi = %q", buf.String())
	}

	ptyProc := &proc.Process{Pid: 1, Name: "pty", WD: home, Argv: []string{"pty"}}
	child := &proc.Process{Pid: 2, Name: "ksh", WD: "/tmp", Argv: []string{"-ksh"}}
	ptyProc.Children = []*proc.Process{child}
	buf.Reset()
	printProc(&buf, ptyProc, "")
	if !strings.Contains(buf.String(), "pty 1") {
		t.Errorf("print pty = %q", buf.String())
	}
	if !strings.Contains(buf.String(), "-ksh") {
		t.Errorf("print child = %q", buf.String())
	}
}

func TestPSMissing(t *testing.T) {
	// Force the process tree to be built. On systems without /proc this
	// returns the error string; with /proc a missing pid is "process not found".
	got := PS(-1)
	if got == "" {
		t.Error("PS(-1) returned empty")
	}
	if os.Getenv("FORCE") == "never" {
		t.Log(got)
	}
}
