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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMe(t *testing.T) {
	name := Me()
	if name != "TestMe" {
		t.Errorf("Me() = %q, want TestMe", name)
	}
}

func TestLoggerMethods(t *testing.T) {
	dir := t.TempDir()
	oldDir, oldDur, oldAge := Dir, Duration, Age
	Dir = dir
	Duration = time.Hour
	Age = time.Hour
	defer func() {
		Dir, Duration, Age = oldDir, oldDur, oldAge
	}()

	lg, err := NewLogger("morelog")
	if err != nil {
		t.Fatal(err)
	}
	lg.Info("hello-info")
	lg.Infof("hello %s", "infof")
	lg.Warnf("hello-warn")
	lg.Errorf("hello-err")
	lg.DumpStack()
	lg.DumpGoroutines()
	_ = Standard()
}

func TestPackageLogFuncs(t *testing.T) {
	// These tolerate a nil package logger via Logger.Outputf.
	Infof("pkg-info")
	Warnf("pkg-warn")
	Errorf("pkg-err")
	DepthInfof(0, "depth-info")
	DepthWarnf(0, "depth-warn")
	DepthErrorf(0, "depth-err")
	Outputf(1, "X", "outputf %d", 1)
}

func TestOutputfNilLogger(t *testing.T) {
	var lg *Logger
	lg.Outputf(1, "I", "nil logger %s", "ok")
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	oldAge := Age
	Age = time.Millisecond
	defer func() { Age = oldAge }()

	lg, err := NewLogger(filepath.Join(dir, "prune"))
	if err != nil {
		t.Fatal(err)
	}

	// A file that looks like a rotated log and is older than Age.
	oldName := "prune-20000101.000000.1"
	oldPath := filepath.Join(dir, oldName)
	if err := ioutil.WriteFile(oldPath, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(oldPath, past, past); err != nil {
		t.Fatal(err)
	}
	// Files that should be ignored by prune.
	ioutil.WriteFile(filepath.Join(dir, "no-dots"), []byte("x"), 0600)
	ioutil.WriteFile(filepath.Join(dir, "not-a-time.xyz"), []byte("x"), 0600)
	ioutil.WriteFile(filepath.Join(dir, "foo-notatime.1"), []byte("x"), 0600)

	lg.prune(dir)

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("prune did not remove expired log %s", oldName)
	}

	lg.prune(filepath.Join(dir, "missing-dir"))
}

func TestNewLoggerOpenFail(t *testing.T) {
	// A path whose parent cannot be created as a directory.
	file := filepath.Join(t.TempDir(), "notdir")
	if err := ioutil.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := NewLogger(filepath.Join(file, "child", "log"))
	if err == nil {
		t.Error("NewLogger through a file succeeded, want error")
	}
}
