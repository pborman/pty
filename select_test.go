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
	"testing"
)

func TestReadline(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old }()
	go func() {
		w.Write([]byte(" yes \n"))
		w.Close()
	}()
	got, err := readline()
	if err != nil {
		t.Fatal(err)
	}
	if got != "yes" {
		t.Errorf("readline = %q, want yes", got)
	}
}

func TestReadlineCR(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old }()
	go func() {
		w.Write([]byte("ok\r"))
		w.Close()
	}()
	got, err := readline()
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Errorf("readline CR = %q, want ok", got)
	}
}

func TestReadYesNo(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin = r
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	defer func() {
		os.Stdin = oldIn
		os.Stdout = oldOut
		devnull.Close()
	}()
	go func() {
		w.Write([]byte("maybe\ny\n"))
		w.Close()
	}()
	ok, err := readYesNo("proceed? ")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("readYesNo y = false")
	}

	r2, w2, _ := os.Pipe()
	os.Stdin = r2
	go func() {
		w2.Write([]byte("N\n"))
		w2.Close()
	}()
	ok, err = readYesNo("proceed? ")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("readYesNo N = true")
	}
}

func TestGetSessionsDead(t *testing.T) {
	defer withTempHome(t)()
	s := MakeSession("dead", "")
	defer s.Remove()
	s.SetPid(1 << 30)
	got := GetSessions()
	if len(got) != 0 {
		t.Errorf("dead session listed: %v", got)
	}
}

func TestReadlineEOF(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old }()
	_, err = readline()
	if err == nil {
		t.Error("readline at EOF succeeded")
	}
}
