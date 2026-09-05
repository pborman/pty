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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)


func stopClient(c *Client) {
	select {
	case <-c.done:
		return
	default:
	}
	select {
	case <-c.quit:
	default:
		close(c.quit)
	}
	<-c.done
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

	cb := &closeBuffer{}
	if err := checkClose(cb); err != nil {
		t.Errorf("checkClose(closeBuffer): %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if !cb.closed {
		t.Error("checkClose did not close closeBuffer")
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
