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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestReadConfigMissing(t *testing.T) {
	defer withTempHome(t)()
	old := config
	defer func() { config = old }()
	config.Forward = nil
	if err := ReadConfig(); err != nil {
		t.Fatal(err)
	}
}

func TestReadConfigOK(t *testing.T) {
	defer withTempHome(t)()
	old := config
	defer func() { config = old }()
	dir := filepath.Join(user.HomeDir, rcdir)
	os.MkdirAll(dir, 0700)
	if err := ioutil.WriteFile(filepath.Join(dir, "config.yaml"), []byte("forward:\n  - FOO\n  - BAR\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ReadConfig(); err != nil {
		t.Fatal(err)
	}
	if len(config.Forward) != 2 || config.Forward[0] != "FOO" || config.Forward[1] != "BAR" {
		t.Errorf("Forward = %q", config.Forward)
	}
}

func TestReadConfigBadYAML(t *testing.T) {
	defer withTempHome(t)()
	old := config
	defer func() { config = old }()
	dir := filepath.Join(user.HomeDir, rcdir)
	os.MkdirAll(dir, 0700)
	if err := ioutil.WriteFile(filepath.Join(dir, "config.yaml"), []byte(":\n  -"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ReadConfig(); err == nil {
		t.Error("bad yaml succeeded")
	}
}

func TestReadConfigUnreadable(t *testing.T) {
	defer withTempHome(t)()
	dir := filepath.Join(user.HomeDir, rcdir)
	os.MkdirAll(dir, 0700)
	path := filepath.Join(dir, "config.yaml")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := ReadConfig(); err == nil {
		t.Error("directory-as-config succeeded")
	}
}
