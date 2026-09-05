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
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pborman/pty/econn"
)

func TestValidSessionName(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want bool
	}{
		{"", true},
		{"ok", true},
		{"log", false},
		{"has/slash", false},
		{"spaces are ok", true},
		{"name_1.2+3", true},
		{"bad\x00nul", false},
		{"[]<>{}!:", true},
	} {
		if got := ValidSessionName(tt.in); got != tt.want {
			t.Errorf("ValidSessionName(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestSessionFiles(t *testing.T) {
	defer withTempHome(t)()
	s := MakeSession("filesess", "sid-1")
	defer s.Remove()

	if s.Name != "filesess" {
		t.Errorf("Name = %q", s.Name)
	}
	if !s.spawn && s.Name == "filesess" {
		// not a +name
	}
	if s.SessionID() != "sid-1" {
		t.Errorf("SessionID = %q, want sid-1", s.SessionID())
	}
	if s.Attach("sid-2").SessionID() != "sid-2" {
		t.Error("Attach did not update id")
	}
	if err := s.SetPid(os.Getpid()); err != nil {
		t.Fatal(err)
	}
	pid, ok := s.Pid()
	if !ok || pid != os.Getpid() {
		t.Errorf("Pid = %d, %v", pid, ok)
	}
	if !s.Ping() {
		t.Error("Ping of our own pid failed")
	}

	if err := s.SetTitle("hello title"); err != nil {
		t.Fatal(err)
	}
	if s.Title() != "hello title" {
		t.Errorf("Title = %q", s.Title())
	}
	if err := s.SetAddr("127.0.0.1:9"); err != nil {
		t.Fatal(err)
	}
	if s.Addr() != "127.0.0.1:9" {
		t.Errorf("Addr = %q", s.Addr())
	}
	if err := s.SetTTYSize(24, 80); err != nil {
		t.Fatal(err)
	}
	if s.TTYSize() != "(80x24)" {
		t.Errorf("TTYSize = %q, want (80x24)", s.TTYSize())
	}
	if s.DebugPath() != filepath.Join(s.path, "debug") {
		t.Errorf("DebugPath = %q", s.DebugPath())
	}

	spawned := MakeSession("+spawned", "")
	defer spawned.Remove()
	if spawned.Name != "spawned" {
		t.Errorf("spawned Name = %q", spawned.Name)
	}
	if !spawned.spawn {
		t.Error("MakeSession(+name) did not set spawn")
	}

	missing := &Session{path: filepath.Join(t.TempDir(), "missing")}
	if _, ok := missing.Pid(); ok {
		t.Error("Pid of missing file succeeded")
	}
	if missing.Title() != "" || missing.Addr() != "" || missing.TTYSize() != "" || missing.SessionID() != "" {
		t.Error("missing files should return empty strings")
	}
	if err := missing.GetSecret(); err == nil {
		t.Error("GetSecret of missing file succeeded")
	}
}

func TestSessionListenDial(t *testing.T) {
	defer withTempHome(t)()
	s := MakeSession("listen-dial", "id")
	defer s.Remove()

	ln, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if s.Addr() == "" {
		t.Fatal("Listen did not record addr")
	}
	pid, ok := s.Pid()
	if !ok || pid != os.Getpid() {
		t.Errorf("Listen pid = %d, %v", pid, ok)
	}
	if !s.Ping() {
		t.Error("Ping after Listen failed")
	}

	got := make(chan []byte, 1)
	errc := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errc <- err
			return
		}
		sc, err := econn.Server(c, s.secret[:])
		if err != nil {
			errc <- err
			return
		}
		buf := make([]byte, 64)
		n, err := io.ReadFull(sc, buf[:4])
		if err != nil {
			errc <- err
			return
		}
		got <- buf[:n]
		sc.Write([]byte("pong"))
		sc.Close()
	}()

	client := MakeSession("listen-dial", "")
	c, err := client.Dial()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errc:
		t.Fatal(err)
	case msg := <-got:
		if string(msg) != "ping" {
			t.Errorf("server got %q", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server")
	}
	reply := make([]byte, 4)
	if _, err := io.ReadFull(c, reply); err != nil {
		t.Fatal(err)
	}
	if string(reply) != "pong" {
		t.Errorf("client got %q", reply)
	}
	c.Close()
}

func TestSessionCommandCount(t *testing.T) {
	defer withTempHome(t)()
	s := MakeSession("cmd-count", "id")
	defer s.Remove()

	ln, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				sc, err := econn.Server(c, s.secret[:])
				if err != nil {
					return
				}
				w := NewMessengerWriter(sc)
				r := NewMessengerReader(sc, func(kind messageKind, data []byte) {
					if kind == askCountMessage {
						w.Sendf(countMessage, "3")
					}
				})
				buf := make([]byte, 256)
				for {
					if _, err := r.Read(buf); err != nil {
						return
					}
				}
			}()
		}
	}()

	if !s.Check() {
		t.Fatal("Check failed")
	}
	if s.cnt != 3 {
		t.Errorf("cnt = %d, want 3", s.cnt)
	}
}

func TestGetSessionsEmpty(t *testing.T) {
	defer withTempHome(t)()
	if got := GetSessions(); len(got) != 0 {
		t.Errorf("GetSessions on empty home = %v", got)
	}
}

func TestMakeRawCookedWhenPipe(t *testing.T) {
	s := &Session{}
	if !isPipe() {
		t.Skip("stdin is a tty; skipping MakeRaw/MakeCooked")
	}
	if err := s.MakeRaw(); err != nil {
		t.Fatal(err)
	}
	if err := s.MakeCooked(); err != nil {
		t.Fatal(err)
	}
}

func TestSessionPSNoPid(t *testing.T) {
	s := &Session{path: t.TempDir()}
	if s.PS() != "" {
		t.Errorf("PS without pid = %q", s.PS())
	}
}

func TestDialTimeout(t *testing.T) {
	defer withTempHome(t)()
	s := MakeSession("no-addr", "")
	defer s.Remove()
	// Addr file is missing, Dial should time out after 5s. That's long
	// for a unit test; we write an unparseable addr instead so Resolve
	// fails quickly after the first loop iteration.
	s.SetAddr("not-an-addr")
	_, err := s.Dial()
	if err == nil {
		t.Error("Dial with bad addr succeeded")
	}
}
