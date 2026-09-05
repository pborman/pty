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
