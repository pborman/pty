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
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// shortDir returns a new directory with a path short enough for unix domain
// sockets (t.TempDir is too long on macOS).
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "pty")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// fakeAgent listens on a unix domain socket at path and answers each line it
// reads with tag, a colon, and the line.
func fakeAgent(t *testing.T, path, tag string) {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					c.Write([]byte(tag + ":" + line))
				}
			}()
		}
	}()
}

// closeForwarder stops the listener of the forwarder registered as name.
func closeForwarder(t *testing.T, name string) {
	t.Cleanup(func() {
		forwardersMu.Lock()
		f := forwarders[name]
		delete(forwarders, name)
		forwardersMu.Unlock()
		if f != nil {
			f.lconn.Close()
		}
	})
}

// ask connects to the unix domain socket at path, sends msg, and returns the
// reply line, or "" and the error if the connection fails or is closed.
func ask(t *testing.T, path, msg string) (string, error) {
	t.Helper()
	c, err := net.Dial("unix", path)
	if err != nil {
		return "", err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte(msg + "\n")); err != nil {
		return "", err
	}
	line, err := bufio.NewReader(c).ReadString('\n')
	return strings.TrimSuffix(line, "\n"), err
}

func TestSetForwarderMissing(t *testing.T) {
	if err := SetForwarder("no-such-socket", "/tmp/remote"); err == nil {
		t.Error("SetForwarder of missing name succeeded")
	}
}

func TestForwarderFollowsRemote(t *testing.T) {
	dir := shortDir(t)
	agent1 := filepath.Join(dir, "agent1")
	agent2 := filepath.Join(dir, "agent2")
	fakeAgent(t, agent1, "one")
	fakeAgent(t, agent2, "two")

	sock := filepath.Join(dir, "fwd")
	closeForwarder(t, "TESTFWD")
	if err := NewForwarder("TESTFWD", sock, agent1); err != nil {
		t.Fatal(err)
	}
	if got, err := ask(t, sock, "hello"); err != nil || got != "one:hello" {
		t.Errorf("first remote: got %q, %v, want %q", got, err, "one:hello")
	}

	// A reattaching client reports a different agent socket.
	if err := SetForwarder("TESTFWD", agent2); err != nil {
		t.Fatal(err)
	}
	if got, err := ask(t, sock, "again"); err != nil || got != "two:again" {
		t.Errorf("second remote: got %q, %v, want %q", got, err, "two:again")
	}
}

func TestForwarderDeadRemote(t *testing.T) {
	dir := shortDir(t)
	sock := filepath.Join(dir, "fwd")
	closeForwarder(t, "TESTDEAD")
	// The client that reported this socket has gone away.
	if err := NewForwarder("TESTDEAD", sock, filepath.Join(dir, "gone")); err != nil {
		t.Fatal(err)
	}
	if got, err := ask(t, sock, "hello"); err == nil {
		t.Errorf("forward to a missing socket answered %q", got)
	}
}

func TestNewForwarderReplacesStaleSocket(t *testing.T) {
	dir := shortDir(t)
	sock := filepath.Join(dir, "fwd")
	if err := os.WriteFile(sock, nil, 0600); err != nil {
		t.Fatal(err)
	}
	closeForwarder(t, "TESTSTALE")
	if err := NewForwarder("TESTSTALE", sock, "/tmp/remote"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(sock); err != nil || fi.Mode()&os.ModeSocket == 0 {
		t.Errorf("forwarder socket: %v, %v", fi, err)
	}
}

func TestNewForwarderListenFails(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "no-such-dir", "fwd")
	if err := NewForwarder("TESTFAIL", sock, "/tmp/remote"); err == nil {
		t.Error("NewForwarder in a missing directory succeeded")
	}
}

func TestForwarderSessionNoRemote(t *testing.T) {
	f := &forwarder{}
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if err := f.session(a, ""); err == nil {
		t.Error("session with no remote succeeded")
	}
}

func TestStartForwarders(t *testing.T) {
	home := shortDir(t)
	old := user.HomeDir
	user.HomeDir = home
	defer func() { user.HomeDir = old }()
	oldForward := config.Forward
	config.Forward = []string{"PTY_TEST_AGENT", "PTY_TEST_UNSET", "PTY_TEST_BAD"}
	defer func() { config.Forward = oldForward }()

	agent := filepath.Join(home, "agent")
	fakeAgent(t, agent, "agent")
	t.Setenv("PTY_TEST_AGENT", agent)
	os.Unsetenv("PTY_TEST_UNSET")
	t.Setenv("PTY_TEST_BAD", "/tmp/whatever")
	closeForwarder(t, "PTY_TEST_AGENT")
	closeForwarder(t, "PTY_TEST_BAD")

	sess := MakeSession("fwd", "")
	defer sess.Remove()
	// Something unremovable where PTY_TEST_BAD's socket goes makes that
	// forwarder fail; the shell must keep the inherited value.
	if err := os.MkdirAll(filepath.Join(sess.path, "PTY_TEST_BAD"+fwdSuffix, "x"), 0700); err != nil {
		t.Fatal(err)
	}
	s := NewShell(sess)
	s.startForwarders()

	env := map[string]string{}
	for _, e := range s.Env {
		if x := strings.IndexByte(e, '='); x > 0 {
			env[e[:x]] = e[x+1:]
		}
	}
	sock := filepath.Join(sess.path, "PTY_TEST_AGENT"+fwdSuffix)
	if env["PTY_TEST_AGENT"] != sock {
		t.Errorf("PTY_TEST_AGENT = %q, want %q", env["PTY_TEST_AGENT"], sock)
	}
	if v, ok := env["PTY_TEST_UNSET"]; ok {
		t.Errorf("unset variable was forwarded as %q", v)
	}
	if env["PTY_TEST_BAD"] != "/tmp/whatever" {
		t.Errorf("PTY_TEST_BAD = %q, want the inherited value", env["PTY_TEST_BAD"])
	}
	// Before any client reports a socket, the inherited one is used.
	if got, err := ask(t, sock, "hi"); err != nil || got != "agent:hi" {
		t.Errorf("forwarded agent: got %q, %v, want %q", got, err, "agent:hi")
	}
}
