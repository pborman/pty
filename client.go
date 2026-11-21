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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/pborman/pty/log"
	"github.com/pborman/pty/mutex"
)

type mBuffer struct {
	kind messageKind
	data []byte
}

// A Client represents an incoming client for a shell.
type Client struct {
	mu      *mutex.Mutex
	name    string
	buffers []mBuffer
	ready   chan struct{}
	done    chan struct{}
	quit    chan struct{}
	once    sync.Once // guards closing quit
	out     io.Writer
	primary bool
	pid     int
}

// NewClient returns a freshly initialized client that writes output to out.
func NewClient(out io.Writer) *Client {
	c := &Client{
		mu:    mutex.New("New Client"),
		out:   out,
		ready: make(chan struct{}, 1),
		done:  make(chan struct{}),
		quit:  make(chan struct{}),
	}
	go c.runout()
	return c
}

// Output implements ShellClient.
func (c *Client) Output(buf []byte) bool {
	return c.Send(dataMessage, buf)
}

// Send implements ShellClient.
func (c *Client) Send(kind messageKind, buf []byte) bool {
	if kind == 0 && len(buf) == 0 {
		return true
	}
	unlock := c.mu.Lock("Send")
	c.buffers = append(c.buffers, mBuffer{kind: kind, data: buf})
	unlock()
	select {
	case c.ready <- struct{}{}:
	default:
	}
	return true
}

// Send implements ShellClient.
func (c *Client) SendLocked(kind messageKind, buf []byte) bool {
	if kind == 0 && len(buf) == 0 {
		return true
	}
	c.buffers = append(c.buffers, mBuffer{kind: kind, data: buf})
	select {
	case c.ready <- struct{}{}:
	default:
	}
	return true
}

// Close implements ShellClient and io.Closer.  It writes whatever output
// is still queued, stops the client's output goroutine and closes c.out.
// It is safe to call more than once and from more than one goroutine; every
// call returns only after the queued output has been written.  Close always
// returns nil.
func (c *Client) Close() error {
	c.once.Do(func() {
		close(c.quit)
		<-c.done
		checkClose(c.out)
	})
	return nil
}

// getFD returns the name and file descriptor number of fd, for logging, or "?"
// and -1 if they are not available.  It must not disturb fd: File would dup the
// descriptor (keeping a socket open after fd is closed) and Fd would put an
// *os.File into blocking mode, so the number is read through SyscallConn.
func getFD(fd interface{}) (string, int) {
	type namer interface {
		Name() string
	}
	name := "?"
	if n, ok := fd.(namer); ok {
		name = n.Name()
	}
	fno := -1
	if sc, ok := fd.(syscall.Conn); ok {
		if rc, err := sc.SyscallConn(); err == nil {
			rc.Control(func(f uintptr) { fno = int(f) })
		}
	}
	return name, fno
}

func checkClose(fd interface{}) error {
	name, fno := getFD(fd)
	if ioc, ok := fd.(io.Closer); ok {
		log.Infof("Closing %d %s %T", fno, name, fd)
		log.DumpStack()
		go func() {
			ioc.Close()
		}()
		return nil
	}
	return nil
}

func (c *Client) nextBuf() mBuffer {
	defer c.mu.Lock("nextBuf")()
	if len(c.buffers) == 0 {
		return mBuffer{}
	}
	buf := c.buffers[0]
	c.buffers = c.buffers[1:]
	return buf
}

func (c *Client) Name() string {
	return c.name
}

func (c *Client) IsActive() bool {
	defer c.mu.Lock("IsActive")()
	// The pid is only set once the client has sent us a ttynameMessage.
	// Clients that are just requesting information do not send a
	// ttynameMessage.
	return c.pid != 0
}

func (c *Client) SetPid(pid int) {
	defer c.mu.Lock("SetPid")()
	c.pid = pid
}

func (c *Client) SetName(name string) {
	defer c.mu.Lock("SetName")()
	c.name = name
}

// runout writes queued output from Output to the client's io.Writer.
// It returns, after one last flush, once Close has closed c.quit.  The
// ready channel is never closed, so Send cannot send on a closed channel.
func (c *Client) runout() {
	defer close(c.done)
	for {
		quitting := false
		select {
		case <-c.ready:
		case <-c.quit:
			quitting = true
		}
		c.flush()
		if quitting {
			return
		}
	}
}

// flush writes all the currently queued buffers to the client.
func (c *Client) flush() {
	for {
		m := c.nextBuf()
		if m.kind == 0 && m.data == nil {
			return
		}
		if m.kind == 0 {
			if _, err := c.out.Write(m.data); err != nil {
				log.Infof("%v", err)
			}
		} else if w, ok := c.out.(*MessengerWriter); ok {
			w.Send(m.kind, m.data)
		}
	}
}

func displayMotd() {
	path := filepath.Join(user.HomeDir, rcdir, "motd")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}
	if _, err := os.Stdout.Write(data); err != nil {
		log.Infof("%v", err)
	}
	fmt.Printf("Press ENTER to continue: ")
	var buf [1]byte
	for {
		if n, _ := os.Stdin.Read(buf[:]); n == 0 || buf[0] == '\n' || buf[0] == '\r' {
			return
		}
	}
}
