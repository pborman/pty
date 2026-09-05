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

package stack

import (
	"bytes"
	"strings"
	"testing"
)

func TestFrameAndLocation(t *testing.T) {
	frame := Frame(1)
	if frame == "" {
		t.Fatal("Frame(1) returned empty")
	}
	if !strings.Contains(frame, "TestFrameAndLocation") {
		t.Errorf("Frame(1) = %q, want it to name this test", frame)
	}
	if !strings.Contains(frame, "stack_test.go") {
		t.Errorf("Frame(1) = %q, want stack_test.go", frame)
	}

	loc := Location(1)
	if loc == "" {
		t.Fatal("Location(1) returned empty")
	}
	if !strings.Contains(loc, "stack_test.go") {
		t.Errorf("Location(1) = %q, want stack_test.go", loc)
	}

	if Frame(10000) != "" {
		t.Errorf("Frame of a too-deep caller should be empty")
	}
	if Location(10000) != "" {
		t.Errorf("Location of a too-deep caller should be empty")
	}
}

func TestDumpAndDumpString(t *testing.T) {
	var buf bytes.Buffer
	Dump(NewLogger(&buf), 1, 3)
	got := buf.String()
	if got == "" {
		t.Fatal("Dump wrote nothing")
	}
	if !strings.Contains(got, "TestDumpAndDumpString") {
		t.Errorf("Dump output %q, want this test name", got)
	}

	// n <= i should still dump at least one frame.
	buf.Reset()
	Dump(NewLogger(&buf), 1, 0)
	if buf.Len() == 0 {
		t.Error("Dump with n <= i wrote nothing")
	}

	s := DumpString(1, 2)
	if s == "" {
		t.Fatal("DumpString returned empty")
	}
	if !strings.Contains(s, "stack.go") && !strings.Contains(s, "stack_test.go") {
		t.Errorf("DumpString = %q, want a stack file name", s)
	}
}

func TestLoggerInfo(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf)
	log.Info("hello", 1)
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("Info wrote %q, want hello", buf.String())
	}
}
