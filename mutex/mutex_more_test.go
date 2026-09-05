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

package mutex

import (
	"bytes"
	"strings"
	"testing"
)

func TestDumpNoDebug(t *testing.T) {
	reset(false)
	m := New("quiet")
	unlock := m.Lock("x")
	var buf bytes.Buffer
	Dump(&buf)
	if buf.Len() != 0 {
		t.Errorf("Dump with debug off wrote %q", buf.String())
	}
	unlock()
	m.logf("should be a no-op %s", "x")
}

func TestLocation(t *testing.T) {
	got := location(-1, "who")
	if !strings.Contains(got, "who") {
		t.Errorf("location(-1) = %q", got)
	}
	got = location(3, "named")
	if !strings.Contains(got, "[3]") || !strings.Contains(got, "named") {
		t.Errorf("location(3) = %q", got)
	}
}

func TestLogfDebug(t *testing.T) {
	reset(true)
	m := New("logged")
	var buf bytes.Buffer
	old := logger
	logger = func(n int, p, format string, v ...interface{}) {
		buf.WriteString(p)
		buf.WriteString(format)
	}
	defer func() { logger = old }()
	unlock := m.Lock("L")
	unlock()
	if buf.Len() == 0 {
		t.Error("debug lock produced no logf output")
	}
}
