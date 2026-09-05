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

package ttyname

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestFileNotTTY(t *testing.T) {
	f, err := os.CreateTemp("", "tty-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	name, err := File(f)
	if name != "" {
		t.Errorf("File(regular) name = %q, want empty", name)
	}
	if !errors.Is(err, ErrNotTTY) {
		t.Errorf("File(regular) err = %v, want ErrNotTTY", err)
	}
}

func TestFilenoNotTTY(t *testing.T) {
	f, err := os.CreateTemp("", "tty-fileno-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	name, err := Fileno(int(f.Fd()))
	if name != "" {
		t.Errorf("Fileno(regular) name = %q, want empty", name)
	}
	if !errors.Is(err, ErrNotTTY) {
		t.Errorf("Fileno(regular) err = %v, want ErrNotTTY", err)
	}
}

func TestFilenoBadFD(t *testing.T) {
	_, err := Fileno(-1)
	if err == nil {
		t.Error("Fileno(-1) succeeded, want error")
	}
}

func TestSearchDirMissing(t *testing.T) {
	name, err := searchDir(filepath.Join(t.TempDir(), "no-such-dir"), nil)
	if name != "" || err != nil {
		t.Errorf("searchDir(missing) = %q, %v; want empty, nil", name, err)
	}
}

func TestSearchDirEmpty(t *testing.T) {
	dir := t.TempDir()
	name, err := searchDir(dir, &syscall.Stat_t{})
	if err != nil {
		t.Fatalf("searchDir: %v", err)
	}
	if name != "" {
		t.Errorf("searchDir(empty) = %q, want empty", name)
	}
}

func TestFileClosed(t *testing.T) {
	f, err := os.CreateTemp("", "tty-closed-")
	if err != nil {
		t.Fatal(err)
	}
	name := f.Name()
	f.Close()
	os.Remove(name)

	_, err = File(f)
	if err == nil {
		t.Error("File(closed) succeeded, want error")
	}
}

func TestStdinIfTTY(t *testing.T) {
	name, err := File(os.Stdin)
	if errors.Is(err, ErrNotTTY) {
		t.Skip("stdin is not a tty")
	}
	if err != nil {
		t.Fatalf("File(stdin): %v", err)
	}
	if name == "" {
		t.Error("File(stdin) returned empty name for a tty")
	}
	t.Logf("stdin tty = %s", name)
}
