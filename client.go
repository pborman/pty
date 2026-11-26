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

	// resync is set on the screen queued by Shell.resync.  Its bytes
	// are not counted in Client.queued, so a screen larger than
	// clientQueueLimit cannot put the client behind again before it
	// has been written.
	resync bool
}

// clientQueueLimit is the most terminal output, in bytes, that is queued for
// a client.  A client whose queue grows past it has stopped reading (its
// terminal was stopped with ^S, the link under it died, or the client was
// suspended).  Its queued output is thrown away and the client is marked
// behind; once its socket accepts writes again it is sent the recorded
// screen, as when it attached.  Tests change it.
var clientQueueLimit = 4 << 20

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

	// queued is the number of bytes of terminal output (dataMessage)
	// in buffers, not counting a resync screen.
	queued int

	// behind is set when the client's terminal output was thrown away
	// because the queue passed clientQueueLimit.  While it is set
	// further terminal output is dropped; control messages are still
	// queued.  The output goroutine clears it, by way of shell.resync,
	// once it has written everything still queued.
	behind bool

	// shell is the shell the client was attached to, which resends the
	// screen to a client that fell behind.  It is set by Shell.Attach.
	shell *Shell
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
	c.queue(kind, buf)
	unlock()
	return true
}

// SendLocked implements ShellClient.  It is Send for a caller already holding
// c.mu.
func (c *Client) SendLocked(kind messageKind, buf []byte) bool {
	if kind == 0 && len(buf) == 0 {
		return true
	}
	c.queue(kind, buf)
	return true
}

// queue adds buf to the client's queue and wakes the output goroutine.  It
// never blocks.  Terminal output is dropped while the client is behind, and
// if it takes the queue past clientQueueLimit all the queued terminal output
// is thrown away and the client is marked behind.  Control messages are
// always queued, and stay in order.  C.mu must be held.
func (c *Client) queue(kind messageKind, buf []byte) {
	if kind == dataMessage {
		if c.behind {
			return
		}
		if c.queued+len(buf) > clientQueueLimit {
			c.dropOutput()
			return
		}
		c.queued += len(buf)
	}
	c.buffers = append(c.buffers, mBuffer{kind: kind, data: buf})
	c.wake()
}

// dropOutput throws away the terminal output in the client's queue, keeping
// the control messages, and marks the client behind.  C.mu must be held.
func (c *Client) dropOutput() {
	log.Infof("client %s is not reading; dropping %d bytes of output", c.name, c.queued)
	kept := c.buffers[:0]
	for _, b := range c.buffers {
		if b.kind != dataMessage {
			kept = append(kept, b)
		}
	}
	// Clear the tail so the dropped buffers can be collected.
	for i := len(kept); i < len(c.buffers); i++ {
		c.buffers[i] = mBuffer{}
	}
	c.buffers = kept
	c.queued = 0
	c.behind = true
	// The output goroutine resyncs the client once the queue is
	// empty, so make sure it looks.
	c.wake()
}

// wake tells the output goroutine there is something to do.
func (c *Client) wake() {
	select {
	case c.ready <- struct{}{}:
	default:
	}
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
		log.Debugf("Closing %d %s %T", fno, name, fd)
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
	c.buffers[0] = mBuffer{}
	c.buffers = c.buffers[1:]
	if buf.kind == dataMessage && !buf.resync {
		c.queued -= len(buf.data)
	}
	return buf
}

// resyncShell returns the shell to resync c from, or nil if c is not behind.
// A client that is behind but was never attached to a shell has no screen to
// be sent, so it is just marked as caught up.
func (c *Client) resyncShell() *Shell {
	defer c.mu.Lock("resyncShell")()
	if !c.behind {
		return nil
	}
	if c.shell == nil {
		c.behind = false
		return nil
	}
	return c.shell
}

func (c *Client) Name() string {
	defer c.mu.Lock("Name")()
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
		c.flush(!quitting)
		if quitting {
			return
		}
	}
}

// flush writes all the currently queued buffers to the client.  If the
// client fell behind and resync is true, once the queue is empty it has the
// shell queue the screen and writes that too.  Resync is false for the
// final flush done by Close: a client that is going away does not need the
// screen.
//
// Because the screen is only asked for once the queue has drained, that is,
// once the socket has accepted everything written before it, a client that
// stays stalled is not sent the screen again and again: while it is stalled
// this goroutine is blocked in a write.
func (c *Client) flush(resync bool) {
	for {
		m := c.nextBuf()
		if m.kind == 0 && m.data == nil {
			if !resync {
				return
			}
			s := c.resyncShell()
			if s == nil {
				return
			}
			s.resync(c)
			continue
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
