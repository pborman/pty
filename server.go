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
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/pborman/pty/log"
	"github.com/pborman/pty/mutex"
)

func (s *Session) shell(debug bool) {
	conn, err := s.Listen()
	if err != nil {
		s.Exitf("server: %v", err)
	}

	shell := NewShell(s)
	if err := shell.Start(debug); err != nil {
		s.Exitf("start: %v\n", err)
	}
	go func() {
		shell.Wait()
		s.Exit(0)
	}()

	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGABRT, syscall.SIGBUS, syscall.SIGQUIT, syscall.SIGSEGV)
	go func() {
		for sig := range ch {
			log.Errorf("signal %v", sig)
			var buf bytes.Buffer
			mutex.Dump(&buf)
			log.Errorf("Mutex Dump:\n%s", buf.String())
			log.DumpGoroutines()
			switch sig {
			case syscall.SIGABRT, syscall.SIGBUS, syscall.SIGSEGV:
				s.Exitf("exiting on signal %d", sig)
			}
		}
	}()

	for {
		c, err := conn.Accept()
		if err != nil {
			s.Exitf("server: %v", err)
		}
		log.Infof("accepted new connection")
		go func() {
			shell.attach(c)
			checkClose(c)
		}()
	}
}

func (s *Shell) attach(c net.Conn) {
	// Attach forwards the shells output to c.
	mw := NewMessengerWriter(c)
	client := NewClient(mw)
	s.Attach(client)
	defer func() { go s.Detach(client) }()

	ech := make(chan error, 1)
	go func() {
		r := NewMessengerReader(c, func(kind messageKind, msg []byte) {
			switch kind {
			case psMessage:
				mw.Send(psMessage, []byte(PS(os.Getpid())))
			case forwardMessage:
				x := bytes.IndexByte(msg, 0)
				if x <= 0 {
					mw.Sendf(serverMessage, "ERROR: BAD FORWARD MESSAGE\r\n")
					return
				}
				name := string(msg[:x])
				socket := string(msg[x+1:])
				if name == "" || socket == "" {
					mw.Sendf(serverMessage, "ERROR: BAD FORWARD MESSAGE\r\n")
					return
				}
				SetForwarder(name, socket)
			case exclusiveMessage:
				unlock := s.mu.Lock("exclusiveMessage")
				var clients []*Client
				for c := range s.clients {
					if c != client {
						clients = append(clients, c)
					}
				}
				unlock()
				for _, oc := range clients {
					s.Detach(oc)
					oc.Output([]byte(fmt.Sprintf("\r\nDetached by client %s\r\n", client.Name())))
					checkClose(oc)
				}
			case askCountMessage:
				mw.Sendf(countMessage, "%d", s.Count())
			case pingMessage:
				mw.Send(ackMessage, msg)
			case ttynameMessage:
				// the ttynameMessage is sent by each client as
				// it attaches, excluding clients that are just
				// asking for information (e.g., pty --list).
				// The message includes the PID of the actual
				// client.
				var pid int
				name := string(msg)
				if x := strings.Index(name, ":"); x > 0 {
					var err error
					pid, err = strconv.Atoi(name[:x])
					if err == nil {
						s.AddPid(client, pid)
						name = name[x+1:]
					}
				}
				if pid == 0 {
					log.Warnf("ttyname with no pid: %s", name)
				}
				client.SetName(name)
			case dumpMessage:
				log.DumpGoroutines()
			case listMessage:
				s.List(client)
			case ttysizeMessage:
				s.Take(client, false)
				if len(msg) != 4 {
					mw.Sendf(serverMessage, "ERROR: SCREEN MSG IS %d BYTES, need 4\r\n", len(msg))
					return
				}
				rows, cols := decodeSize(msg)
				unlock := s.mu.Lock("ttysizeMessage")
				if rows == s.rows && cols == s.cols {
					unlock()
					return
				}
				s.rows = rows
				s.cols = cols
				unlock()
				if err := s.Setsize(rows, cols); err != nil {
					mw.Sendf(serverMessage, "ERROR: SETSIZE: %v\r\n", err)
				}
			case saveMessage:
				var err error
				unlock := s.mu.Lock("saveMessage")
				if s.eb.inalt {
					err = ioutil.WriteFile(string(msg), s.eb.alt, 0600)
				} else {
					err = ioutil.WriteFile(string(msg), s.eb.normal, 0600)
				}
				unlock()
				if err != nil {
					mw.Sendf(serverMessage, "ERROR: saving screen: %v\n", err)
				} else {
					mw.Sendf(serverMessage, "screen saved to %s\r\n", msg)
				}
			case escapeMessage:
				unlock := s.mu.Lock("escapeMessage")
				s.eb.sendEscapes(mw, strings.ToLower(string(msg)) == "alt")
				unlock()
			default:
				mw.Sendf(serverMessage, "ERROR: UNSUPPORTED KIND %d\r\n", kind)
			}
		})
		var data [32 * 1024]byte
		for {
			var werr error
			r, rerr := r.Read(data[:])
			if r > 0 {
				s.Take(client, true)
				_, werr = s.Write(data[:r])
			}
			if rerr != nil {
				log.Warnf("Read from client: %v", rerr)
				ech <- rerr
				break
			}
			if werr != nil {
				log.Warnf("Write to shell: %v", werr)
				ech <- werr
				break
			}
		}
	}()
	select {
	case <-s.done:
	case err := <-ech:
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
	}
	checkClose(c)
}

// spawnServer spawns a server process.  When we come back we will end up
// s.run().
func (s *Session) Spawn(debugFile string, foreground bool) {
	if s.Check() {
		s.Exitf("Session %q already exists", s.Name)
	}
	if foreground {
		s.run(debugFile)
		return
	}
	// We prepend session with a "+" to indicate we
	// want our child to fork another child and then
	// s.Exit.
	args := []string{"--internal", "+" + s.Name}
	if debugFile != "" {
		args = append(args, "--internal_debug", s.Name+debugSuffix)
	}

	cmd := exec.Command(os.Args[0], args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{}

	if err := cmd.Start(); err != nil {
		s.Exitf("starting subprocess: %v", err)
	}
	fmt.Printf("Started session %s\n", s.Name)
	s.started = true
	go func() {
		err := cmd.Wait()
		if err != nil {
			fmt.Printf("internal shell s.Exited: %v", err)
		}
	}()
}

func (s *Session) run(debugFile string) {
	if s.spawn {
		args := []string{"--internal", s.Name}
		if debugFile != "" {
			args = append(args, "--internal_debug", debugFile)
		}
		cmd := exec.Command(os.Args[0], args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}
		if err := cmd.Start(); err != nil {
			log.Errorf("rexec failed: %v", err)
			s.Exitf("rexec failed: %v\n", err)
		}
		return
	}
	if debugFile != "" {
		debugInit(debugFile)
	}
	s.shell(false)
	return
}
