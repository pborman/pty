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
	"io/ioutil"
	"net"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestTakeConcurrent has several clients take the session at once, while
// the shell sends them output and lists them, as when several people type
// into one session.  Take used to lock its own client before the shell, so
// two clients taking at once deadlocked the server.
func TestTakeConcurrent(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("takes", "")
	defer sess.Remove()
	s := NewShell(sess)

	var clients []*Client
	for i := 0; i < 4; i++ {
		c := NewClient(ioutil.Discard)
		c.SetName(fmt.Sprintf("c%d", i))
		s.Attach(c)
		clients = append(clients, c)
	}
	deadlocked := false
	defer func() {
		if deadlocked {
			return // stopping the clients would hang too
		}
		for _, c := range clients {
			stopClient(c)
		}
	}()

	var wg sync.WaitGroup
	for _, c := range clients {
		c := c
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				s.Take(c, i%2 == 0)
				c.SetName(c.Name())
			}
		}()
	}
	wg.Add(1)
	go func() {
		// What Shell.runout and List do: s.mu, then each client's.
		defer wg.Done()
		for i := 0; i < 500; i++ {
			unlock := s.mu.Lock("test")
			for c := range s.clients {
				c.Output([]byte("x"))
				c.Name()
			}
			unlock()
			s.List(clients[0])
		}
	}()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		deadlocked = true
		t.Fatal("deadlock: concurrent Takes did not finish")
	}

	// Exactly one client is primary, and it is the shell's primary.
	n := 0
	for _, c := range clients {
		unlock := c.mu.Lock("test")
		if c.primary {
			n++
			if s.primary != c {
				t.Errorf("%s is primary but the shell's primary is %v", c.name, s.primary)
			}
		}
		unlock()
	}
	if n != 1 {
		t.Errorf("%d primary clients, want 1", n)
	}
}

// TestTakeSendsMessages checks the messages Take sends: primary to the
// client that takes, preempt to the one it takes from, and nothing when the
// client is already primary.
func TestTakeSendsMessages(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("takemsgs", "")
	defer sess.Remove()
	s := NewShell(sess)

	c1, c2 := NewClient(ioutil.Discard), NewClient(ioutil.Discard)
	s.Attach(c1)
	s.Attach(c2)
	defer stopClient(c1)
	defer stopClient(c2)
	kinds := func(c *Client) []messageKind {
		unlock := c.mu.Lock("test")
		defer unlock()
		var k []messageKind
		for _, b := range c.buffers {
			k = append(k, b.kind)
		}
		c.buffers = nil
		return k
	}
	// Stop the clients' output goroutines so the messages stay queued,
	// and drop what Attach sent.
	stopClient(c1)
	stopClient(c2)
	kinds(c1)
	kinds(c2)

	s.Take(c1, false)
	if k := kinds(c1); fmt.Sprint(k) != fmt.Sprint([]messageKind{primaryMessage}) {
		t.Errorf("c1 got %v, want primary", k)
	}
	s.Take(c1, false)
	if k := kinds(c1); len(k) != 0 {
		t.Errorf("c1 taking again got %v, want nothing", k)
	}
	s.Take(c2, false)
	if k := kinds(c1); fmt.Sprint(k) != fmt.Sprint([]messageKind{preemptMessage}) {
		t.Errorf("c1 got %v, want preempt", k)
	}
	if k := kinds(c2); fmt.Sprint(k) != fmt.Sprint([]messageKind{primaryMessage}) {
		t.Errorf("c2 got %v, want primary", k)
	}

	// Once the primary client detaches, it can take the session again.
	s.Detach(c2)
	s.Attach(c2)
	kinds(c2)
	s.Take(c2, false)
	if k := kinds(c2); fmt.Sprint(k) != fmt.Sprint([]messageKind{primaryMessage}) {
		t.Errorf("c2 after reattach got %v, want primary", k)
	}
}

// TestPs checks that ps gets the answer to its request, and that an answer
// with no request waiting, such as one that comes after ps gave up, is
// dropped.  Run with -race, it also checks that psChan is guarded.
func TestPs(t *testing.T) {
	var mu sync.Mutex
	var buf bytes.Buffer
	w := NewMessengerWriter(lockedWriter{&mu, &buf})

	psAnswer([]byte("nobody asked")) // must not block or panic

	for i := 0; i < 50; i++ {
		want := fmt.Sprintf("answer %d", i)
		got := make(chan []byte)
		go func() { got <- ps(w) }()
		// Answer as soon as the request is pending.  Extra answers
		// race with ps returning and must be dropped.
		for {
			psMu.Lock()
			pending := psChan != nil
			psMu.Unlock()
			if pending {
				break
			}
			time.Sleep(time.Millisecond)
		}
		psAnswer([]byte(want))
		psAnswer([]byte("late"))
		select {
		case data := <-got:
			if string(data) != want {
				t.Fatalf("ps = %q, want %q", data, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("ps did not return")
		}
	}
	psMu.Lock()
	defer psMu.Unlock()
	if psChan != nil {
		t.Error("psChan left set after ps returned")
	}
}

type lockedWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// lockFree reports whether s.mu can be taken within a second.
func lockFree(s *Shell) bool {
	got := make(chan struct{})
	go func() {
		s.mu.Lock("test")()
		close(got)
	}()
	select {
	case <-got:
		return true
	case <-time.After(time.Second):
		return false
	}
}

// stalledConn starts s serving a connection whose client end, returned,
// reads nothing until the test reads it.  A net.Pipe has no buffer, so
// every write the server makes to it blocks until the test reads.
func stalledConn(t *testing.T, s *Shell) net.Conn {
	t.Helper()
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		s.attach(server)
		close(done)
	}()
	t.Cleanup(func() {
		client.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("attach did not return")
		}
	})
	return client
}

// TestEscapeMessageStalledClient asks for the escape sequence list from a
// client that is not reading.  The list used to be written to the client's
// socket with the shell's lock held, so the write blocked and froze the
// session for every client.
func TestEscapeMessageStalledClient(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("escstall", "")
	defer sess.Remove()
	s := NewShell(sess)
	unlock := s.mu.Lock("test")
	s.eb.Write([]byte("\033[1mbold\033[0m \033[31mred\033[m\r\n"))
	var want bytes.Buffer
	s.eb.sendEscapes(&want, false)
	unlock()

	conn := stalledConn(t, s)
	if _, err := NewMessengerWriter(conn).Send(escapeMessage, []byte("normal")); err != nil {
		t.Fatal(err)
	}
	// Give the server time to reach its write to the stalled client.
	time.Sleep(100 * time.Millisecond)
	if !lockFree(s) {
		t.Fatal("the shell lock is held while writing to a stalled client")
	}

	// The client is sent the same list as before, as plain output.
	got := make([]byte, want.Len())
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("reading the escape list: %v", err)
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Errorf("escape list = %q, want %q", got, want.Bytes())
	}
}

// TestSaveMessageSlowFile saves the screen to a FIFO that no one is reading.
// The file used to be written with the shell's lock held, so opening it
// blocked the whole session.
func TestSaveMessageSlowFile(t *testing.T) {
	defer withTempHome(t)()
	sess := MakeSession("savestall", "")
	defer sess.Remove()
	s := NewShell(sess)
	unlock := s.mu.Lock("test")
	s.eb.Write([]byte("saved text\r\n"))
	want := append([]byte{}, s.eb.normal...)
	unlock()

	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	conn := stalledConn(t, s)
	if _, err := NewMessengerWriter(conn).Send(saveMessage, []byte(fifo)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if !lockFree(s) {
		t.Fatal("the shell lock is held while writing the save file")
	}

	// Read the FIFO, and the server's reply, so the server can finish.
	go NewMessengerReader(conn, func(messageKind, []byte) {}).Read(make([]byte, 1024))
	got, err := ioutil.ReadFile(fifo)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("saved %q, want %q", got, want)
	}
}
