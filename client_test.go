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
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func stopClient(c *Client) {
	select {
	case <-c.done:
		return
	default:
	}
	c.Close()
}

func TestClientOutput(t *testing.T) {
	var buf bytes.Buffer
	c := NewClient(&buf)
	if !c.Output([]byte("hello")) {
		t.Fatal("Output returned false")
	}
	if !c.Send(0, nil) {
		t.Error("Send(0, nil) should succeed")
	}
	time.Sleep(50 * time.Millisecond)
	stopClient(c)
	if buf.String() != "hello" {
		t.Errorf("got %q, want hello", buf.String())
	}
}

func TestClientCloseHang(t *testing.T) {
	var buf bytes.Buffer
	c := NewClient(&buf)
	c.Output([]byte("x"))
	done := make(chan struct{})
	go func() {
		c.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Client.Close hung waiting for runout")
		stopClient(c)
		<-done
	}
}

func TestClientNamePidActive(t *testing.T) {
	var buf bytes.Buffer
	c := NewClient(&buf)
	defer stopClient(c)
	if c.Name() != "" {
		t.Errorf("Name = %q", c.Name())
	}
	if c.IsActive() {
		t.Error("new client should not be active")
	}
	c.SetName("tty")
	c.SetPid(os.Getpid())
	if c.Name() != "tty" {
		t.Errorf("Name = %q", c.Name())
	}
	if !c.IsActive() {
		t.Error("client with pid should be active")
	}
}

func TestClientSendLockedAndTyped(t *testing.T) {
	var buf bytes.Buffer
	mw := NewMessengerWriter(&buf)
	c := NewClient(mw)
	unlock := c.mu.Lock("test")
	c.SendLocked(0, nil)
	c.SendLocked(pingMessage, []byte("abcd"))
	unlock()
	time.Sleep(50 * time.Millisecond)
	stopClient(c)
	if buf.Len() == 0 {
		t.Error("typed SendLocked wrote nothing")
	}
}

func TestGetFDAndCheckClose(t *testing.T) {
	f, err := os.CreateTemp("", "pty-fd-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	name, fd := getFD(f)
	if fd < 0 {
		t.Errorf("getFD fd = %d", fd)
	}
	if name == "" || name == "?" {
		t.Errorf("getFD name = %q", name)
	}
	if err := checkClose(f); err != nil {
		t.Errorf("checkClose: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	n, fd := getFD(struct{}{})
	if n != "?" || fd != -1 {
		t.Errorf("getFD(struct) = %q, %d", n, fd)
	}
	if err := checkClose(struct{}{}); err != nil {
		t.Errorf("checkClose(struct): %v", err)
	}

	// checkClose closes in the background, so wait for it.
	cb := &closeBuffer{}
	if err := checkClose(cb); err != nil {
		t.Errorf("checkClose(closeBuffer): %v", err)
	}
	for deadline := time.Now().Add(5 * time.Second); !cb.isClosed(); {
		if time.Now().After(deadline) {
			t.Fatal("checkClose did not close closeBuffer")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDisplayMotdMissing(t *testing.T) {
	defer withTempHome(t)()
	displayMotd()
}

func TestDisplayMotdPresent(t *testing.T) {
	defer withTempHome(t)()
	dir := filepath.Join(user.HomeDir, rcdir)
	os.MkdirAll(dir, 0700)
	if err := ioutil.WriteFile(filepath.Join(dir, "motd"), []byte("hello motd\n"), 0600); err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("\n"))
	w.Close()
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin = r
	or, ow, _ := os.Pipe()
	os.Stdout = ow
	displayMotd()
	ow.Close()
	os.Stdin, os.Stdout = oldIn, oldOut
	out, _ := ioutil.ReadAll(or)
	if !bytes.Contains(out, []byte("hello motd")) {
		t.Errorf("motd output = %q", out)
	}
}

// checkClose must really close a socket: getFD used to dup the descriptor
// with File, which kept the connection open until the dup was collected.
func TestCheckCloseClosesSocket(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	peer, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()

	if _, fd := getFD(c); fd < 0 {
		t.Errorf("getFD of a TCP conn = %d", fd)
	}
	checkClose(c)
	peer.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := peer.Read(make([]byte, 1)); err != io.EOF {
		t.Errorf("peer read after checkClose = %v, want EOF", err)
	}
}

// gateWriter is a writer whose writes block until open is closed, like a
// socket whose peer has stopped reading.  Started is closed when the first
// write begins.
type gateWriter struct {
	open    chan struct{}
	started chan struct{}
	once    sync.Once
	mu      sync.Mutex
	buf     bytes.Buffer
}

func newGateWriter() *gateWriter {
	return &gateWriter{open: make(chan struct{}), started: make(chan struct{})}
}

func (g *gateWriter) Write(p []byte) (int, error) {
	g.once.Do(func() { close(g.started) })
	<-g.open
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.buf.Write(p)
}

func (g *gateWriter) bytes() []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]byte{}, g.buf.Bytes()...)
}

// queuedData returns the number of bytes of terminal output queued for c and
// the number of control messages queued.
func queuedData(c *Client) (data, msgs int) {
	defer c.mu.Lock("test")()
	for _, b := range c.buffers {
		if b.kind == dataMessage {
			data += len(b.data)
		} else {
			msgs++
		}
	}
	return data, msgs
}

// TestClientStalledQueueBounded attaches a client whose socket accepts
// nothing and has the shell print far more than clientQueueLimit, as with
// yes(1) running while a terminal is stopped with ^S.  The client's queue
// must stay bounded, its control messages must all be kept, and once it reads
// again it must be sent, in order, what it was writing when it stalled, the
// control messages, the recorded screen, and then output that came after.
func TestClientStalledQueueBounded(t *testing.T) {
	defer withTempHome(t)()
	defer func(n int) { clientQueueLimit = n }(clientQueueLimit)
	clientQueueLimit = 4096

	sess := MakeSession("stalled", "")
	defer sess.Remove()
	s := NewShell(sess)

	// What Shell.runout does with output from the shell.
	output := func(data string) {
		defer s.mu.Lock("test")()
		s.eb.Write([]byte(data))
		buf := []byte(data)
		for c := range s.clients {
			c.Output(buf)
		}
	}
	output("before\r\n")

	g := newGateWriter()
	c := NewClient(NewMessengerWriter(g))
	defer closeClient(t, c)
	s.Attach(c)
	select {
	case <-g.started:
	case <-time.After(5 * time.Second):
		t.Fatal("client never wrote the start message")
	}
	// The output goroutine is now blocked writing the start message.
	// The attach screen is queued behind it, and is thrown away with
	// the rest of the output.

	var wantMsgs []string
	for i := 0; i < 2000; i++ {
		output(fmt.Sprintf("line %d\r\n", i))
		if i%100 == 0 {
			m := fmt.Sprintf("msg %d", i)
			wantMsgs = append(wantMsgs, m)
			c.Send(serverMessage, []byte(m))
		}
		if data, _ := queuedData(c); data > clientQueueLimit {
			t.Fatalf("after line %d, %d bytes queued, limit %d", i, data, clientQueueLimit)
		}
	}
	// The output was much more than the limit, so the client is
	// behind, nothing but the control messages is queued, and no screen
	// is queued while it stays stalled.
	unlock := c.mu.Lock("test")
	behind := c.behind
	unlock()
	if !behind {
		t.Fatal("stalled client is not marked behind")
	}
	if data, msgs := queuedData(c); data != 0 || msgs != len(wantMsgs) {
		t.Errorf("stalled client has %d data bytes and %d messages queued, want 0 and %d", data, msgs, len(wantMsgs))
	}

	unlock = s.mu.Lock("test")
	wantScreen := s.screen()
	unlock()

	// Let the client read again and wait for it to be resynced.
	close(g.open)
	for deadline := time.Now().Add(5 * time.Second); ; {
		unlock := c.mu.Lock("test")
		behind := c.behind
		unlock()
		if !behind {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("client was never resynced")
		}
		time.Sleep(time.Millisecond)
	}
	output("after\r\n")
	closeClient(t, c)

	// Split what the client was sent into data and messages.
	// A NUL in data marks where each message came.
	gotMsgs := []string{}
	var data []byte
	r := NewMessengerReader(bytes.NewReader(g.bytes()), func(kind messageKind, msg []byte) {
		gotMsgs = append(gotMsgs, kind.String()+" "+string(msg))
		data = append(data, 0)
	})
	var rbuf [1024]byte
	for {
		n, err := r.Read(rbuf[:])
		data = append(data, rbuf[:n]...)
		if err != nil {
			break
		}
	}
	want := []byte{0} // the start message
	for range wantMsgs {
		want = append(want, 0)
	}
	want = append(want, wantScreen...)
	want = append(want, "after\r\n"...)
	if !bytes.Equal(data, want) {
		t.Errorf("client was sent:\n%q\nwant:\n%q", data, want)
	}
	for i := range wantMsgs {
		wantMsgs[i] = "serverMessage " + wantMsgs[i]
	}
	wantMsgs = append([]string{"startMessage "}, wantMsgs...)
	if fmt.Sprint(gotMsgs) != fmt.Sprint(wantMsgs) {
		t.Errorf("messages:\n%q\nwant\n%q", gotMsgs, wantMsgs)
	}
}

// TestClientKeepingUp checks that a client that reads everything gets all of
// the output, unchanged, even when it adds up to far more than
// clientQueueLimit.
func TestClientKeepingUp(t *testing.T) {
	defer func(n int) { clientQueueLimit = n }(clientQueueLimit)
	clientQueueLimit = 4096

	var buf closeBuffer
	c := NewClient(&buf)
	var want []byte
	for i := 0; i < 5000; i++ {
		line := []byte(fmt.Sprintf("line %d\r\n", i))
		want = append(want, line...)
		c.Output(line)
		// Give the output goroutine time to keep up.
		for deadline := time.Now().Add(5 * time.Second); ; {
			if data, _ := queuedData(c); data < clientQueueLimit/2 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("client output goroutine is stuck")
			}
			time.Sleep(time.Millisecond)
		}
	}
	closeClient(t, c)
	buf.mu.Lock()
	defer buf.mu.Unlock()
	if !bytes.Equal(buf.buf, want) {
		t.Errorf("client got %d bytes, want %d", len(buf.buf), len(want))
	}
}

// shortCloseTimeout sets clientCloseTimeout to d for the rest of the test.
func shortCloseTimeout(t *testing.T, d time.Duration) {
	old := clientCloseTimeout.Load()
	clientCloseTimeout.Store(int64(d))
	t.Cleanup(func() { clientCloseTimeout.Store(old) })
}

// closeWithin calls c.Close and fails the test if it does not return within
// d.
func closeWithin(t *testing.T, c *Client, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		c.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("Client.Close did not return within %v", d)
	}
}

// Close on a client whose writer never takes anything returns once
// clientCloseTimeout has passed.
func TestClientCloseStalled(t *testing.T) {
	shortCloseTimeout(t, 50*time.Millisecond)
	g := newGateWriter()
	defer close(g.open) // let the output goroutine go
	c := NewClient(g)
	c.Output([]byte("stuck"))
	<-g.started
	closeWithin(t, c, 2*time.Second)
}

// Close on a client whose peer never reads closes the connection, which
// ends the write the output goroutine was blocked in.
func TestClientCloseStalledConn(t *testing.T) {
	shortCloseTimeout(t, 50*time.Millisecond)
	server, peer := net.Pipe()
	defer peer.Close()
	c := NewClient(NewMessengerWriter(server))
	c.Output([]byte("stuck"))
	c.Send(serverMessage, []byte("also stuck"))
	closeWithin(t, c, 2*time.Second)
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		t.Fatal("the output goroutine is still blocked after Close")
	}
	peer.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := peer.Read(make([]byte, 1)); err != io.EOF {
		t.Errorf("peer read after Close = %v, want EOF", err)
	}
}

// A client that is reading gets everything queued before Close.
func TestClientCloseFlushes(t *testing.T) {
	server, peer := net.Pipe()
	defer peer.Close()
	c := NewClient(NewMessengerWriter(server))
	want := bytes.Repeat([]byte("0123456789"), 10000)
	c.Output(want)
	c.Output([]byte("end"))
	got := make(chan []byte)
	go func() {
		data, _ := ioutil.ReadAll(NewMessengerReader(peer, nil))
		got <- data
	}()
	closeWithin(t, c, 5*time.Second)
	if data := <-got; string(data) != string(want)+"end" {
		t.Errorf("peer got %d bytes, want %d", len(data), len(want)+3)
	}
}

// OutputFinal reaches a client that is behind, whose other output is being
// dropped.
func TestClientOutputFinalBehind(t *testing.T) {
	defer func(n int) { clientQueueLimit = n }(clientQueueLimit)
	clientQueueLimit = 16
	g := newGateWriter()
	c := NewClient(g)
	c.Output([]byte("first"))
	<-g.started // the output goroutine is blocked writing "first"
	c.Output(bytes.Repeat([]byte("x"), 20))
	c.Output([]byte("lost"))
	c.OutputFinal([]byte("bye"))
	c.Output(bytes.Repeat([]byte("y"), 20))
	close(g.open)
	closeWithin(t, c, 5*time.Second)
	if got := string(g.bytes()); got != "firstbye" {
		t.Errorf("client got %q, want %q", got, "firstbye")
	}
}
