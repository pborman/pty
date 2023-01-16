package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

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
	out     io.Writer
	primary bool
}

// NewClient returns a freshly initialized client that writes output to out.
func NewClient(out io.Writer) *Client {
	log.Infof("Added new client")
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
	checkClose(c.out)
}

func getFD(fd interface{}) (string, int) {
	type filer interface {
		File() (*os.File, error)
	}
	type fder interface {
		Fd() uintptr
	}
	type namer interface {
		Name() string
	}
	name := "?"
	if n, ok := fd.(namer); ok {
		name = n.Name()
	}
	ffd := fd
	if f, ok := fd.(filer); ok {
		f1, err := f.File()
		if err != nil {
			log.Errorf("closing %v", err)
			log.DumpStack()
			return name, -1
		}
		ffd = f1
	}
	if f, ok := ffd.(fder); ok {
		return name, int(f.Fd())
	}
	return name, -1
}
func checkClose(fd interface{}) error {
	name, fno := getFD(fd)
	if ioc, ok := fd.(io.Closer); ok {
		log.Infof("Closing %d %s %T", fno, name, fd)
		log.DumpStack()
		return ioc.Close()
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

func (c *Client) SetName(name string) {
	defer c.mu.Lock("SetName")()
	c.name = name
}

// runout writes queued output from Output to the client's io.Writer.
func (c *Client) runout() {
checkStdin()
	defer func() {
checkStdin()
		log.Errorf("returning from runout")
checkStdin()
		if p := recover(); p != nil {
			log.Errorf("Panic: %v", p)
			log.DumpGoroutines()
			panic(p)
		}
	}()
	defer close(c.done)
checkStdin()
	ready := c.ready
checkStdin()
	for {
checkStdin()
		select {
		case _, ok := <-ready:
checkStdin()
			if !ok {
				return
			}
		case <-c.quit:
checkStdin()
			return
		}
checkStdin()
		for {
checkStdin()
			m := c.nextBuf()
checkStdin()
			if m.kind == 0 && m.data == nil {
checkStdin()
				break
			}
checkStdin()
			if m.kind == 0 {
checkStdin()
				if _, err := c.out.Write(m.data); err != nil {
checkStdin()
log.Infof("%v", err)
				}
checkStdin()
			} else if w, ok := c.out.(*MessengerWriter); ok {
checkStdin()
				w.Send(m.kind, m.data)
checkStdin()
			}
		}
	}
checkStdin()
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
