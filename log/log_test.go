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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLog runs the package logger long enough to rotate at least once.
func TestLog(t *testing.T) {
	oldDir, oldDur, oldAge, oldLogger := Dir, Duration, Age, logger
	defer func() {
		Dir, Duration, Age, logger = oldDir, oldDur, oldAge, oldLogger
	}()
	Dir = t.TempDir()
	Duration = 1 * time.Second
	Age = 4 * time.Second
	logger = nil
	if err := Init("tlog"); err != nil {
		t.Fatal(err)
	}
	// Log file names have one second resolution, so a rotation needs
	// the clock to reach a new second before the tick.
	deadline := time.After(2500 * time.Millisecond)
	tick := time.NewTicker(time.Second / 4)
	defer tick.Stop()
	for i := 0; ; i++ {
		select {
		case <-tick.C:
			Infof("message %d", i)
			continue
		case <-deadline:
		}
		break
	}
	logger.Close()

	files, err := filepath.Glob(filepath.Join(Dir, "tlog-*"))
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(data)
	}
	for _, want := range []string{"message 0", "switched from log"} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("logs do not contain %q:\n%s", want, all.String())
		}
	}
}
