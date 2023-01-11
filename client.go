package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

type mBuffer struct {
	kind messageKind
	data []byte
}

// A Client represents an incoming client for a shell.
type Client struct {
	mu      sync.Mutex
	name    string
	buffers []mBuffer
	ready   chan struct{}
	done    chan struct{}
	quit    chan struct{}
	out     io.Writer
	primary bool
}

// NewClient returns a freshly initialized client that writes output to out.
func NewClient(out io.Writer) *Client {
	c := &Client{
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
	c.mu.Lock()
	c.buffers = append(c.buffers, mBuffer{kind: kind, data: buf})
	c.mu.Unlock()
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

// Close implements ShellClient.
func (c *Client) Close() {
	if c.ready == nil {
		return
	}
	select {
	case c.ready <- struct{}{}:
	default:
	}
	close(c.ready)
	c.ready = nil
	<-c.done
	if ioc, ok := c.out.(io.Closer); ok {
		ioc.Close()
	}
}

func (c *Client) nextBuf() mBuffer {
	c.mu.Lock()
	defer c.mu.Unlock()
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

func (c *Client) SetName(name string) {
	c.mu.Lock()
	c.name = name
	c.mu.Unlock()
}

// runout writes queued output from Output to the client's io.Writer.
func (c *Client) runout() {
	defer close(c.done)
	ready := c.ready
	for {
		select {
		case _, ok := <-ready:
			if !ok {
				return
			}
		case <-c.quit:
			return
		}
		for {
			m := c.nextBuf()
			if m.kind == 0 && m.data == nil {
				break
			}
			if m.kind == 0 {
				c.out.Write(m.data)
			} else if w, ok := c.out.(*MessengerWriter); ok {
				w.Send(m.kind, m.data)
			}
		}
	}
}

func displayMotd() {
	path := filepath.Join(user.HomeDir, rcdir, "motd")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}
	os.Stdout.Write(data)
	fmt.Printf("Press ENTER to continue: ")
	var buf [1]byte
	for {
		if n, _ := os.Stdin.Read(buf[:]); n == 0 || buf[0] == '\n' || buf[0] == '\r' {
			return
		}
	}
}
