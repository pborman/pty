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

package log

import (
	"bytes"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestIsRegister(t *testing.T) {
	for _, tt := range []struct {
		line string
		want bool
	}{
		{"rax 0x0", true},
		{"rbx 0x1", true},
		{"r15 0x2", true},
		{"rip 0x3", true},
		{"rflags 0x4", true},
		{"cs 0x5", true},
		{"hello world", false},
		{"RAX 0x0", false},
		{"", false},
		{"r8 1", true},
	} {
		if got := isRegister([]byte(tt.line)); got != tt.want {
			t.Errorf("isRegister(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestNextLine(t *testing.T) {
	line, rest := nextLine([]byte("abc\ndef"))
	if string(line) != "abc\n" || string(rest) != "def" {
		t.Errorf("nextLine = %q, %q", line, rest)
	}
	line, rest = nextLine([]byte("nolf"))
	if string(line) != "nolf" || rest != nil {
		t.Errorf("nextLine(nolf) = %q, %q", line, rest)
	}

	defer func() {
		if recover() == nil {
			t.Error("nextLine(nil) did not panic")
		}
	}()
	nextLine(nil)
}

func TestCleanStack(t *testing.T) {
	buf := make([]byte, 1<<16)
	n := runtime.Stack(buf, true)
	got := CleanStack(buf[:n])
	// Local frames from this package should survive cleaning.
	if !bytes.Contains(got, []byte("goroutine")) {
		t.Errorf("CleanStack dropped goroutine headers:\n%s", got)
	}

	pwd, _ := os.Getwd()
	homeDir := os.Getenv("HOME")
	dump := "goroutine 1 [running]:\n" +
		"github.com/pborman/pty/log.TestCleanStack()\n" +
		"\t" + pwd + "/dump_test.go:99 +0x1\n" +
		"runtime.main()\n" +
		"\t/usr/local/go/src/runtime/proc.go:1 +0x1\n" +
		"goroutine 2 [chan receive]:\n" +
		"github.com/pborman/pty/log.other()\n" +
		"\t" + homeDir + "/src/pty/log/other.go:1 +0x1\n"
	cleaned := CleanStack([]byte(dump))
	if !bytes.Contains(cleaned, []byte("TestCleanStack")) {
		t.Errorf("CleanStack lost local frame:\n%s", cleaned)
	}
	if bytes.Contains(cleaned, []byte("runtime.main")) {
		t.Errorf("CleanStack kept non-local frame:\n%s", cleaned)
	}
	if !bytes.Contains(cleaned, []byte("other.go")) {
		t.Errorf("CleanStack lost HOME frame:\n%s", cleaned)
	}

	if out := CleanStack(nil); len(out) != 0 {
		t.Errorf("CleanStack(nil) = %q, want empty", out)
	}
	if out := CleanStack([]byte("not a stack\n")); len(out) != 0 {
		t.Errorf("CleanStack(plain) = %q, want empty", out)
	}
}

func TestLast2(t *testing.T) {
	for _, tt := range []struct {
		in, want string
	}{
		{"a/b/c/d.go", "c/d.go"},
		{"a/b.go", "a/b.go"},
		{"file.go", "file.go"},
		{"/abs/pkg/file.go", "pkg/file.go"},
	} {
		if got := last2(tt.in); got != tt.want {
			t.Errorf("last2(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCleanStackEmptyGoroutine(t *testing.T) {
	dump := "goroutine 1 [running]:\n\ngoroutine 2 [running]:\n"
	got := CleanStack([]byte(dump))
	if strings.Contains(string(got), "goroutine 1") && !strings.Contains(string(got), "Test") {
		// A goroutine with no local frames should be omitted.
	}
	if len(got) != 0 && bytes.Contains(got, []byte("goroutine 1")) && !bytes.Contains(got, []byte("/")) {
		t.Errorf("unexpected cleaned stack:\n%s", got)
	}
}
