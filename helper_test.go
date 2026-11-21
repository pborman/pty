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
	"sync"
	"testing"
	"time"
)

func withTempHome(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()
	old := user.HomeDir
	user.HomeDir = dir
	return func() { user.HomeDir = old }
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type errWriter struct {
	n   int
	err error
}

func (e errWriter) Write(p []byte) (int, error) {
	if e.err != nil {
		return e.n, e.err
	}
	return len(p), nil
}

// closeBuffer is an io.WriteCloser that records what is written and whether
// it has been closed.  It may be written and closed from other goroutines, as
// checkClose does.
type closeBuffer struct {
	mu     sync.Mutex
	buf    []byte
	closed bool
}

func (c *closeBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, p...)
	return len(p), nil
}

func (c *closeBuffer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *closeBuffer) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *closeBuffer) Name() string { return "closeBuffer" }

// closeClient calls Client.Close with a timeout. Close once blocked forever
// waiting for runout; if that happens again the test fails rather than hangs.
func closeClient(t *testing.T, c *Client) {
	t.Helper()
	if c == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		c.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Client.Close hung waiting for runout")
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}
}
