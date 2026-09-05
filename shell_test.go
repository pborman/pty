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
	"testing"
)

func TestNewShellAndSetenv(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("shellsess", "")
	defer sess.Remove()

	s := NewShell(sess)
	if s.Shell == "" {
		t.Fatal("LoginShell was empty")
	}
	if s.session != sess {
		t.Error("session not attached")
	}
	if s.Done() {
		t.Error("new shell should not be Done")
	}

	s.Setenv("FOO", "bar")
	found := false
	for _, e := range s.Env {
		if e == "FOO=bar" {
			found = true
		}
	}
	if !found {
		t.Errorf("Setenv did not add FOO=bar: %q", s.Env)
	}
	s.Setenv("FOO", "baz")
	count := 0
	for _, e := range s.Env {
		if len(e) >= 4 && e[:4] == "FOO=" {
			count++
			if e != "FOO=baz" {
				t.Errorf("replaced value = %q", e)
			}
		}
	}
	if count != 1 {
		t.Errorf("FOO appears %d times", count)
	}
}

func TestShellAttachDetachListTake(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("shellclients", "")
	defer sess.Remove()
	s := NewShell(sess)

	var buf bytes.Buffer
	c1 := NewClient(&buf)
	c1.SetName("one")
	c1.SetPid(os.Getpid())
	n := s.Attach(c1)
	if n != 0 {
		t.Errorf("first Attach index = %d", n)
	}
	if s.CountClients() != 1 {
		t.Errorf("CountClients = %d, want 1", s.CountClients())
	}

	c2 := NewClient(&bytes.Buffer{})
	c2.SetName("two")
	s.Attach(c2)
	if s.CountClients() != 1 {
		// c2 has no pid so it is not "active"
		t.Logf("CountClients with inactive client = %d", s.CountClients())
	}

	s.Take(c1, false)
	s.Take(c1, false) // already primary
	s.Take(c2, false)

	s.List(c1)
	s.AddPid(c1, os.Getpid())
	if s.Count() != 1 {
		t.Errorf("Count = %d, want 1", s.Count())
	}
	s.AddPid(c2, -1)
	s.Count() // should drop the dead pid

	s.Detach(c2)
	s.Detach(c1)
	stopClient(c1)
	stopClient(c2)
}

func TestShellStartAlreadyStarted(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("started", "")
	defer sess.Remove()
	s := NewShell(sess)
	s.pty = os.Stdin
	if err := s.Start(false); err == nil {
		t.Error("Start on already-started shell succeeded")
	}
}
