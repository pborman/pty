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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kr/pty"
	"github.com/pborman/pty/log"
	"github.com/pborman/pty/mutex"
)

var LoginShell string

func init() {
	for _, shell := range []string{
		"/usr/bin/ksh",
		"/bin/ksh",
		"/bin/sh",
	} {
		if _, err := os.Stat(shell); err == nil {
			LoginShell = shell
			return
		}
	}
	exitf("no available shells")
}

type messageKind int

const (
	dataMessage = messageKind(iota)
	ttysizeMessage
	ttynameMessage
	serverMessage
	startMessage
	waitMessage
	listMessage
	countMessage
	askCountMessage
	exclusiveMessage
	saveMessage
	escapeMessage
	preemptMessage // sent when another client takes control
	primaryMessage // sent when we become primary
	forwardMessage // NAME\0socket
	psMessage
	pingMessage
	ackMessage
	dumpMessage // Cause the server to dump
)

var messageNames = map[messageKind]string{
	dataMessage:      "dataMessage",
	ttysizeMessage:   "ttysizeMessage",
	ttynameMessage:   "ttynameMessage",
	serverMessage:    "serverMessage",
	startMessage:     "startMessage",
	waitMessage:      "waitMessage",
	listMessage:      "listMessage",
	countMessage:     "countMessage",
	askCountMessage:  "askCountMessage",
	exclusiveMessage: "exclusiveMessage",
	saveMessage:      "saveMessage",
	escapeMessage:    "escapeMessage",
	preemptMessage:   "preemptMessage",
	primaryMessage:   "primaryMessage",
	forwardMessage:   "forwardMessage",
	psMessage:        "psMessage",
	pingMessage:      "pingMessage",
	ackMessage:       "ackMessage",
	dumpMessage:      "dumpMessage",
}

func (m messageKind) String() string {
	if s, ok := messageNames[m]; ok {
		return s
	}
	return fmt.Sprintf("message-%d", m)
}

// These escape sequences are used to switch screen buffers.
//
// For more details: http://invisible-island.net/xterm/ctlseqs/ctlseqs.html
const (
	scasb   = "\033[?1049h" // save cursor, switch to alternate screen buffer
	nsbrc   = "\033[?1049l" // switch to normal screen buffer, restore cursor
	cls     = nsbrc + "\033[J\033[3J\033[J"
	edb0    = "\033[J"  // Erase below
	edb1    = "\033[0J" // Erase below
	eda     = "\033[1J" // Erase above
	edall   = "\033[2J" // Erase all
	edsaved = "\033[3J" // Erase Saved Lines
	home    = "\033[H"
	sendSSH = "\033[z"
)

// A ShellClient can be attached to a Shell.
type ShellClient interface {
	// Output is called with each new block of bytes to be sent to the
	// client.  When a client is attached to a shell, Output with the
	// current buffer.  The buffer passed to output should be treated as
	// immutable.  Output returns true if successful or false if the client
	// has failed and should be removed from the list of clients.
	//
	// Output must not block.
	Output([]byte) bool

	// Send sends a message to the client.
	Send(int, []byte) bool

	// SendLocked sends a message to the client, which is already locked.
	SendLocked(int, []byte) bool

	// Close is called when no further output will be sent to the client.
	// Close should block until the client has flushed all data that was
	// sent via Output.
	Close()
}

// A Shell represents an actual running shell.  There may be zero or more
// clients attached to the shell.  The Shell parameter is the name of the shell
// to start when Start is called.  Args are the arguments to pass to the shell.
// If not empty, Args must start with arg0.
type Shell struct {
	Shell      string
	Args       []string
	Env        []string
	cmd        *exec.Cmd
	pty        *os.File
	session    *Session
	started    chan struct{}
	done       chan struct{}
	wg         sync.WaitGroup
	mu         *mutex.Mutex
	clients    map[*Client]struct{}
	pids       map[int]*Client
	eb         *EscapeBuffer
	exiting    bool
	rows, cols int
}

// NewShell returns a newly initialized, but not started, Shell.  By default,
// Shell.Shell is set to LoginShell and Args is set to the basename of the
// LoginShell with a "-" prepended (to indicate it is a login shell).
func NewShell(session *Session) *Shell {
	s := &Shell{
		mu:      mutex.New("Shell " + session.Name),
		started: make(chan struct{}),
		done:    make(chan struct{}),
		clients: map[*Client]struct{}{},
		pids:    map[int]*Client{},
		Shell:   LoginShell,
		Args:    []string{"-" + path.Base(LoginShell)},
		Env:     os.Environ(),
		eb:      NewEscapeBuffer(0),
		session: session,
	}
	s.eb.AddSequence(sendSSH, func(eb *EscapeBuffer) bool {
		return false
	})
	s.eb.AddSequence(scasb, func(eb *EscapeBuffer) bool {
		eb.inalt = true
		return false
	})
	s.eb.AddSequence(nsbrc, func(eb *EscapeBuffer) bool {
		eb.inalt = false
		return false
	})
	s.eb.AddSequence(edsaved, func(eb *EscapeBuffer) bool {
		if !eb.inalt {
			eb.normal = eb.normal[:0]
			eb.normal = append(eb.normal, []byte(nsbrc)...)
			eb.normal = append(eb.normal, []byte(edall)...)
		}
		return false
	})
	s.eb.AddSequence(edall, func(eb *EscapeBuffer) bool {
		if eb.inalt {
			eb.alt = eb.alt[:0]
			eb.alt = append(eb.alt, []byte(home)...)
			eb.alt = append(eb.alt, []byte(edall)...)
		}
		return false
	})
	return s
}

// AddPid adds pid to the list of client pids.
func (s *Shell) AddPid(client *Client, pid int) {
	defer s.mu.Lock("AddPid")()
	s.pids[pid] = client
}

func (s *Shell) Count() int {
	defer s.mu.Lock("Count")()
	for pid, client := range s.pids {
		if syscall.Kill(pid, 0) != nil {
			delete(s.pids, pid)
			s.detach(client)
		}
	}
	return len(s.pids)
}

// Setenv replaces or adds the specified key value pair to the shell's
// environment.  Setenv has no effect once Start is called.
func (s *Shell) Setenv(key, value string) {
	value = key + "=" + value
	key = value[:len(key)+1]
	defer s.mu.Lock("Setenv")()
	for i, v := range s.Env {
		if strings.HasPrefix(v, key) {
			s.Env[i] = value
			return
		}
	}
	s.Env = append(s.Env, value)
}

func (s *Shell) Attach(c *Client) int {
	log.Infof("attach new client")
	defer s.mu.Lock("Attach")()
	c.Send(startMessage, nil)
	buf := append([]byte(cls), s.eb.normal...)
	if !c.Output(buf) {
		log.Infof("new client write failure")
		return len(s.clients)
	}
	if s.eb.inalt {
		c.Output([]byte(scasb))
		buf := append([]byte{}, s.eb.alt...)
		if !c.Output(buf) {
			log.Infof("new client write failure")
			return len(s.clients)
		}
	}
	// Don't take ownership here, wait
	// until the first input from the client
	// arrived.
	s.wg.Add(1)
	s.clients[c] = struct{}{}
	return len(s.clients) - 1
}

func (s *Shell) CountClients() int {
	defer s.mu.Lock("CountClients")()
	cnt := 0
	for c := range s.clients {
		if c.IsActive() {
			cnt++
		}
	}
	return cnt
}

func (s *Shell) Take(c *Client, requestSize bool) {
	defer c.mu.Lock("Take1")()
	if c.primary {
		return
	}
	defer s.mu.Lock("Take2")()
	log.Infof("client %s takes the session", c.Name())
	for oc := range s.clients {
		if oc == c {
			continue
		}
		unlock := oc.mu.Lock("Take3")
		if oc.primary {
			oc.primary = false
			oc.SendLocked(preemptMessage, nil)
		}
		unlock()
	}
	c.primary = true
	c.SendLocked(primaryMessage, nil) // The client should reforward things
}

func (s *Shell) Detach(c *Client) {
	log.Infof("detach client %s", c.Name())
	defer s.mu.Lock("Detach")()
	s.detach(c)
}

func (s *Shell) detach(c *Client) {
	if _, ok := s.clients[c]; ok {
		delete(s.clients, c)
		s.wg.Done()
	}
}

func (s *Shell) Write(buf []byte) (int, error) {
	n, err := s.pty.Write(buf)
	if err != nil {
		log.DepthErrorf(1, "pty write: %v", err)
	}
	return n, err
}

func (s *Shell) Wait() {
	<-s.done
	return
}

func (s *Shell) Done() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *Shell) runout() {
	var buf [8192]byte
	r, err := s.pty.Read(buf[:])
	close(s.started)
	for {
		if func() bool {
			unlock := s.mu.Lock("runout1")
			defer func() { unlock() }()

			if r > 0 {
				s.eb.Write(buf[:r])
				nbuf := append([]byte{}, buf[:r]...)
				for c := range s.clients {
					if !c.Output(nbuf) {
						log.Infof("write to client %s failed", c.Name())
						s.detach(c)
					}
				}
			}
			if err != nil {
				log.Infof("deleting all clients")
				for c := range s.clients {
					c := c
					s.detach(c)
					go func() {
						checkClose(c)
					}()
				}
				unlock()
				s.wg.Wait()
				unlock = s.mu.Lock("runout2")
				close(s.done)
				unlock()
				return true
			}
			return false
		}() {
			return
		}
		r, err = s.pty.Read(buf[:])
		if err != nil {
			log.Errorf("pty read: %v", err)
		}
	}
}

func (s *Shell) Start(debug bool) error {
	s.Setenv("_PTY_NAME", s.session.Name)
	s.Setenv("_PTY_SHELL", "true")
	s.Setenv("_PTY_SOCKET", s.session.Addr())
	if s.pty != nil {
		return errors.New("shell already started")
	}
	for _, name := range config.Forward {
		value := os.Getenv(name)
		if value == "" {
			continue
		}
		sock := name + fwdSuffix
		s.Setenv(name, sock)
		if err := NewForwarder(name, sock); err != nil {
			s.session.Exitf("forwarder[%s]: %s\n", name, err)
		}
	}
	if s.cmd == nil {
		s.cmd = exec.Command(s.Shell)
		s.cmd.Args = s.Args
		s.cmd.Env = s.Env
	}

	fd, tty, err := pty.Open()
	if err != nil {
		return err
	}

	defer func() {
		checkClose(tty)
	}()
	s.cmd.Stdout = tty
	s.cmd.Stdin = tty
	s.cmd.Stderr = tty
	s.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}
	err = s.cmd.Start()
	if err != nil {
		checkClose(fd)
		return err
	}
	s.pty = fd

	// Give the shell a chance to change the tty settings
	time.Sleep(time.Second / 10)
	go s.runout()
	<-s.started
	go func() {
		err = s.cmd.Wait()
		s.Exit()
	}()
	return nil
}

func (s *Shell) Exit() {
	unlock := s.mu.Lock("Exit")
	if s.exiting {
		unlock()
		return
	}
	s.exiting = true
	clients := s.clients
	unlock()
	for c := range clients {
		// Perhaps this should be locked.
		if s.eb.inalt {
			c.Output([]byte(nsbrc))
		}
		checkClose(c)
	}
	s.session.Exit(0)
}

func (s *Shell) List(me *Client) {
	defer s.mu.Lock("List")()
	lines := make([]string, 0, len(s.clients))
	for c := range s.clients {
		name := c.Name()
		if c == me {
			name += " *"
		}
		lines = append(lines, name)
	}
	sort.Strings(lines)
	var buf bytes.Buffer
	for _, line := range lines {
		fmt.Fprintf(&buf, "%s\r\n", line)
	}
	me.Send(serverMessage, buf.Bytes())
}

func (s *Shell) Setsize(rows, cols int) error {
	return setsize(s.pty, rows, cols)
}
