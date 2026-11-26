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
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// The kind values are the wire protocol.  If this test fails, a value was
// changed: put it back.  A new kind gets the next value and a line here.
func TestMessageKindValues(t *testing.T) {
	for _, tt := range []struct {
		kind messageKind
		want int
	}{
		{dataMessage, 0},
		{ttysizeMessage, 1},
		{ttynameMessage, 2},
		{serverMessage, 3},
		{startMessage, 4},
		{waitMessage, 5},
		{listMessage, 6},
		{countMessage, 7},
		{askCountMessage, 8},
		{exclusiveMessage, 9},
		{saveMessage, 10},
		{escapeMessage, 11},
		{preemptMessage, 12},
		{primaryMessage, 13},
		{forwardMessage, 14},
		{psMessage, 15},
		{pingMessage, 16},
		{ackMessage, 17},
		{dumpMessage, 18},
		{clearMessage, 19},
		{versionMessage, 20},
	} {
		if int(tt.kind) != tt.want {
			t.Errorf("%v = %d, want %d", tt.kind, int(tt.kind), tt.want)
		}
	}
	// Every kind has a name, and no two kinds share a value.
	if len(messageNames) != 21 {
		t.Errorf("messageNames has %d kinds, want 21", len(messageNames))
	}
	for k := messageKind(0); k <= versionMessage; k++ {
		if _, ok := messageNames[k]; !ok {
			t.Errorf("kind %d has no name", k)
		}
	}
}

func TestParseVersion(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want int
	}{
		{"", 0},
		{"0", 0},
		{"1", 1},
		{"42", 42},
		{"7 extra fields", 7},
		{"3;x=y", 3},
		{"x", 0},
		{"-1", 0},
		{" 2", 0},
		{"99999999999999999999999", 0},
	} {
		if got := parseVersion([]byte(tt.in)); got != tt.want {
			t.Errorf("parseVersion(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
	if got := parseVersion(versionPayload()); got != protocolVersion {
		t.Errorf("parseVersion(versionPayload()) = %d, want %d", got, protocolVersion)
	}
}

func TestPeerVersion(t *testing.T) {
	var p peerVersion
	if v, known := p.Version(); v != 0 || known {
		t.Errorf("new peerVersion = %d, %v", v, known)
	}
	if p.Supports(0) {
		t.Error("an unknown version supports version 0")
	}
	p.setDefault()
	if v, known := p.Version(); v != 0 || !known {
		t.Errorf("after setDefault = %d, %v", v, known)
	}
	if !p.Supports(0) || p.Supports(1) {
		t.Error("version 0 supports the wrong versions")
	}
	p = peerVersion{}
	if !p.set(3) {
		t.Error("first set returned false")
	}
	if p.set(4) {
		t.Error("second set returned true")
	}
	p.setDefault()
	if v, _ := p.Version(); v != 4 {
		t.Errorf("setDefault changed the version to %d", v)
	}
	if !p.Supports(versionClear) || p.Supports(5) {
		t.Error("version 4 supports the wrong versions")
	}
}

// A kindRecorder records the kinds of the messages it is handed.
type kindRecorder struct {
	mu    sync.Mutex
	kinds []messageKind
}

func (k *kindRecorder) add(kind messageKind) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.kinds = append(k.kinds, kind)
}

func (k *kindRecorder) list() []messageKind {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]messageKind{}, k.kinds...)
}

// oldServer serves c as a server from before protocol versions and before
// clearMessage did: it never sends a version and answers any kind it does
// not know with an error, as Shell.attach did.  It records the kinds it is
// sent.
func oldServer(c net.Conn, got *kindRecorder) {
	w := NewMessengerWriter(c)
	r := NewMessengerReader(c, func(kind messageKind, msg []byte) {
		got.add(kind)
		switch {
		case kind == ttynameMessage:
			w.Send(startMessage, nil)
			w.Write([]byte("old screen"))
		case kind == pingMessage:
			w.Send(ackMessage, msg)
		case kind < clearMessage:
		default:
			w.Sendf(serverMessage, "ERROR: UNSUPPORTED KIND %d\r\n", kind)
		}
	})
	io.Copy(io.Discard, r)
}

// testClient is the client side of main, from dialing on: the reader
// goroutine, the ttynameMessage, and waiting for startMessage.
type testClient struct {
	w      *MessengerWriter
	server peerVersion
	mu     sync.Mutex
	out    bytes.Buffer // terminal output and serverMessages
}

func startTestClient(t *testing.T, c net.Conn, sess *Session) *testClient {
	t.Helper()
	tc := &testClient{w: NewMessengerWriter(c)}
	ready := make(chan struct{})
	write := func(buf []byte) {
		tc.mu.Lock()
		tc.out.Write(buf)
		tc.mu.Unlock()
	}
	go readServer(c, tc.w, ready, &tc.server, sess, write)
	tc.w.Sendf(ttynameMessage, "%d:%s", 1, "test-tty")
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("no startMessage from the server")
	}
	return tc
}

// sync waits for the server to have handled everything tc has sent.
func (tc *testClient) sync(t *testing.T) {
	t.Helper()
	if err := ping(tc.w); err != nil {
		t.Fatal(err)
	}
}

func (tc *testClient) output() string {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.out.String()
}

// A new client talking to a server that has no version must send it
// neither its version nor a clearMessage, and must hear no complaint.
func TestClientOldServer(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("oldserver", "")
	defer sess.Remove()

	server, client := net.Pipe()
	defer client.Close()
	var got kindRecorder
	go oldServer(server, &got)

	tc := startTestClient(t, client, sess)
	if v, known := tc.server.Version(); v != 0 || !known {
		t.Errorf("server version = %d, %v, want 0, true", v, known)
	}
	sendClear(tc.w, &tc.server, []byte("$ \033[1;3H"))
	tc.sync(t)

	for _, k := range got.list() {
		if k == versionMessage || k == clearMessage {
			t.Errorf("old server was sent %v", k)
		}
	}
	if out := tc.output(); strings.Contains(out, "UNSUPPORTED") || out != "old screen" {
		t.Errorf("client output %q, want %q", out, "old screen")
	}
}

// recordConn is a net.Conn that keeps a copy of everything read from it.
type recordConn struct {
	net.Conn
	mu  sync.Mutex
	buf bytes.Buffer
}

func (r *recordConn) Read(p []byte) (int, error) {
	n, err := r.Conn.Read(p)
	r.mu.Lock()
	r.buf.Write(p[:n])
	r.mu.Unlock()
	return n, err
}

func (r *recordConn) bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]byte{}, r.buf.Bytes()...)
}

// serverClient returns the one client attached to s.
func serverClient(t *testing.T, s *Shell) *Client {
	t.Helper()
	defer s.mu.Lock("test")()
	if len(s.clients) != 1 {
		t.Fatalf("shell has %d clients, want 1", len(s.clients))
	}
	for c := range s.clients {
		return c
	}
	return nil
}

// A new client and a new server learn each other's version, the server's
// coming first on the wire, and the client sends clears.
func TestClientNewServer(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("newserver", "")
	defer sess.Remove()
	s := NewShell(sess)
	s.eb.Write([]byte("history"))

	server, client := net.Pipe()
	defer client.Close()
	go s.attach(server)
	rc := &recordConn{Conn: client}

	tc := startTestClient(t, rc, sess)
	if v, known := tc.server.Version(); v != protocolVersion || !known {
		t.Errorf("server version = %d, %v, want %d, true", v, known, protocolVersion)
	}
	screen := "$ \033[1;3H"
	sendClear(tc.w, &tc.server, []byte(screen))
	tc.sync(t)

	if v := serverClient(t, s).PeerVersion(); v != protocolVersion {
		t.Errorf("server has the client at version %d, want %d", v, protocolVersion)
	}
	unlock := s.mu.Lock("test")
	normal := string(s.eb.normal)
	unlock()
	if normal != clearPrefix+screen {
		t.Errorf("server recorded %q after the clear, want %q", normal, clearPrefix+screen)
	}
	if out := tc.output(); strings.Contains(out, "UNSUPPORTED") {
		t.Errorf("client output %q", out)
	}

	// The very first bytes are the version, and startMessage follows.
	raw := rc.bytes()
	first := fmt.Sprintf("\000%c\000\000\000%c%d", versionMessage, len(versionPayload()), protocolVersion)
	if !bytes.HasPrefix(raw, []byte(first)) {
		t.Errorf("connection starts %q, want %q", raw, first)
	}
	var kinds kindRecorder
	io.Copy(io.Discard, NewMessengerReader(bytes.NewReader(raw), func(kind messageKind, _ []byte) {
		kinds.add(kind)
	}))
	if k := kinds.list(); len(k) < 2 || k[0] != versionMessage || k[1] != startMessage {
		t.Errorf("server sent %v, want versionMessage then startMessage first", k)
	}
}

// A client that never sends its version, as old clients do, is version 0
// to the server, and is still served.
func TestServerOldClient(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("oldclient", "")
	defer sess.Remove()
	s := NewShell(sess)
	attachPipeClient(t, s, "old") // never sends a version
	c := serverClient(t, s)
	if v := c.PeerVersion(); v != 0 {
		t.Errorf("old client has version %d, want 0", v)
	}
	if c.peer.Supports(1) {
		t.Error("old client supports version 1")
	}
}

// Session.Command connections, as used by Check and listing, work with a
// server that sends its version first.  (TestSessionCommandCount covers a
// server that does not.)
func TestSessionCommandNewServer(t *testing.T) {
	defer withTempHome(t)()
	s := MakeSession("cmd-new", "id")
	defer s.Remove()
	shell := NewShell(s)

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
				sc, err := s.secure(c)
				if err != nil {
					return
				}
				shell.attach(sc)
				sc.Close()
			}()
		}
	}()

	client := MakeSession("cmd-new", "")
	out := captureStdout(t, func() {
		msg, err := client.Command(askCountMessage, countMessage)
		if err != nil || msg != "0" {
			t.Errorf("Command = %q, %v, want \"0\", nil", msg, err)
		}
		if !client.Check() {
			t.Error("Check failed")
		}
	})
	if out != "" {
		t.Errorf("Command printed %q", out)
	}
}
