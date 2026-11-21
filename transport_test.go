//   Copyright 2026 Paul Borman
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
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pborman/pty/econn"
)

// withShortHome points user.HomeDir at a fresh directory with a short path,
// so ~/.pty/@NAME/addr fits in sun_path and Listen can use a unix socket.
// t.TempDir is not used because on macOS it is long enough to force TCP.
func withShortHome(t *testing.T) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "pty")
	if err != nil {
		t.Fatal(err)
	}
	old := user.HomeDir
	user.HomeDir = dir
	t.Cleanup(func() {
		user.HomeDir = old
		os.RemoveAll(dir)
	})
}

// withLongHome points user.HomeDir at a directory whose path is too long for
// a unix socket, which forces Listen to fall back to TCP.
func withLongHome(t *testing.T) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), strings.Repeat("d", 110))
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	old := user.HomeDir
	user.HomeDir = dir
	t.Cleanup(func() { user.HomeDir = old })
}

// roundTrip has a fresh client Dial the session served by ln and checks that
// data flows both ways through econn.
func roundTrip(t *testing.T, s *Session, ln net.Listener) {
	t.Helper()
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
		defer sc.Close()
		buf := make([]byte, 4)
		if _, err := io.ReadFull(sc, buf); err != nil {
			errc <- err
			return
		}
		_, err = sc.Write(append([]byte("re:"), buf...))
		errc <- err
	}()

	c, err := MakeSession(s.Name, "").Dial()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 7)
	if _, err := io.ReadFull(c, reply); err != nil {
		t.Fatal(err)
	}
	if string(reply) != "re:ping" {
		t.Errorf("client got %q, want %q", reply, "re:ping")
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

// fileMode returns the mode of name in s's session directory.
func fileMode(t *testing.T, s *Session, name string) os.FileMode {
	t.Helper()
	fi, err := os.Lstat(filepath.Join(s.path, name))
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode()
}

func TestNetAddr(t *testing.T) {
	withShortHome(t)
	s := MakeSession("netaddr", "")
	defer s.Remove()
	for _, tt := range []struct {
		contents, network, addr string
	}{
		{"", "", ""},
		{"/tmp/x/socket", "unix", "/tmp/x/socket"},
		{"127.0.0.1:9", "tcp", "127.0.0.1:9"},
	} {
		if err := s.SetAddr(tt.contents); err != nil {
			t.Fatal(err)
		}
		network, addr := s.netAddr()
		if network != tt.network || addr != tt.addr {
			t.Errorf("addr %q: netAddr = %q, %q, want %q, %q", tt.contents, network, addr, tt.network, tt.addr)
		}
		if s.Addr() != tt.addr {
			t.Errorf("addr %q: Addr = %q, want %q", tt.contents, s.Addr(), tt.addr)
		}
	}
}

func TestListenUsesUnixSocket(t *testing.T) {
	withShortHome(t)
	s := MakeSession("unix", "")
	defer s.Remove()
	ln, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if m := fileMode(t, s, "socket"); m&os.ModeSocket == 0 {
		t.Fatalf("socket mode = %v, want a socket", m)
	}
	if m := fileMode(t, s, "addr"); !m.IsRegular() {
		t.Fatalf("addr mode = %v, want a regular file", m)
	}
	path := filepath.Join(s.path, "socket")
	if network, addr := s.netAddr(); network != "unix" || addr != path {
		t.Errorf("netAddr = %q, %q, want unix, %q", network, addr, path)
	}
	roundTrip(t, s, ln)
}

func TestListenFallsBackToTCP(t *testing.T) {
	withLongHome(t)
	s := MakeSession("tcp", "")
	defer s.Remove()
	ln, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if network, addr := s.netAddr(); network != "tcp" || addr != ln.Addr().String() {
		t.Errorf("netAddr = %q, %q, want tcp, %q", network, addr, ln.Addr())
	}
	if _, err := os.Lstat(filepath.Join(s.path, "socket")); !os.IsNotExist(err) {
		t.Errorf("socket left behind after falling back: %v", err)
	}
	roundTrip(t, s, ln)
}

func TestListenReplacesStaleTCPAddr(t *testing.T) {
	withShortHome(t)
	s := MakeSession("stale-tcp", "")
	defer s.Remove()
	if err := s.SetAddr("127.0.0.1:9"); err != nil {
		t.Fatal(err)
	}
	ln, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if network, _ := s.netAddr(); network != "unix" {
		t.Fatalf("network = %q, want unix", network)
	}
	roundTrip(t, s, ln)
}

// deadServer leaves behind what a server that died without cleaning up
// leaves: a socket file nothing listens on, and addr naming it.
func deadServer(t *testing.T, s *Session) {
	t.Helper()
	ln, err := s.listenUnix()
	if err != nil {
		t.Fatal(err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	ln.Close()
}

func TestListenReplacesStaleSocket(t *testing.T) {
	withShortHome(t)
	s := MakeSession("stale-sock", "")
	defer s.Remove()
	deadServer(t, s)
	ln, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if network, _ := s.netAddr(); network != "unix" {
		t.Fatalf("network = %q, want unix", network)
	}
	roundTrip(t, s, ln)
}

func TestCommandStaleUnixSocket(t *testing.T) {
	withShortHome(t)
	s := MakeSession("dead", "")
	deadServer(t, s)
	if _, err := s.Command(askCountMessage, countMessage); err != removedErr {
		t.Errorf("Command on a dead socket = %v, want %v", err, removedErr)
	}
	if _, err := os.Stat(s.path); !os.IsNotExist(err) {
		t.Errorf("session directory not removed: %v", err)
	}
}

func TestListenSocketUnusable(t *testing.T) {
	withShortHome(t)
	s := MakeSession("bad-socket", "")
	defer s.Remove()
	// A non-empty directory where the socket goes cannot be removed or
	// bound over, so Listen must fall back to TCP.
	if err := os.MkdirAll(filepath.Join(s.path, "socket", "x"), 0700); err != nil {
		t.Fatal(err)
	}
	ln, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if network, _ := s.netAddr(); network != "tcp" {
		t.Fatalf("network = %q, want tcp", network)
	}
	roundTrip(t, s, ln)
}

func TestListenAddrUnwritable(t *testing.T) {
	withShortHome(t)
	s := MakeSession("bad-addr", "")
	defer s.Remove()
	// With a non-empty directory at addr neither transport can publish
	// its address, so Listen fails and must not leave the socket behind.
	if err := os.MkdirAll(filepath.Join(s.path, "addr", "x"), 0700); err != nil {
		t.Fatal(err)
	}
	if ln, err := s.listenUnix(); err == nil {
		ln.Close()
		t.Fatal("listenUnix with an unwritable addr succeeded")
	}
	if _, err := os.Lstat(filepath.Join(s.path, "socket")); !os.IsNotExist(err) {
		t.Errorf("socket left behind: %v", err)
	}
	if ln, err := s.Listen(); err == nil {
		ln.Close()
		t.Error("Listen with an unwritable addr succeeded")
	}
}

func TestProbeUnixNoListener(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s")
	if len(path) >= 104 {
		dir, err := os.MkdirTemp("/tmp", "pty")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		path = filepath.Join(dir, "s")
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	ln.SetUnlinkOnClose(false)
	ln.Close()
	if err := probeUnix(ln, path); err == nil {
		t.Error("probeUnix of a closed listener succeeded")
	}
}

func TestNetAddrUnreadable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read mode 0 files")
	}
	withShortHome(t)
	s := MakeSession("unreadable", "")
	defer s.Remove()
	if err := s.SetAddr("127.0.0.1:9"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(s.path, "addr"), 0); err != nil {
		t.Fatal(err)
	}
	if network, addr := s.netAddr(); network != "" || addr != "" {
		t.Errorf("netAddr of unreadable addr = %q, %q", network, addr)
	}
}
