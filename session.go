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
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pborman/pty/log"
	"golang.org/x/crypto/ssh/terminal"
)

// A Session represent a (possibly not created) session.  It is used in both the
// client code and the server code. The only two elements that must be set are
// the Name and the path.  A server will call Listen on the session while a
// client will call Dial on the session.
type Session struct {
	Name    string // Name of the session (client and server)
	cnt     int    // Set by Check to the current number of clients
	path    string // The directory for this session
	spawn   bool   // respawn rather than execing a shell
	started bool   // set true if we started the session

	// Below are fields only used by a client
	ostate *terminal.State
	tilde  byte
}

const validBytes = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-.+!=:[]<>{}"

func ValidSessionName(name string) bool {
	if name == "log" {
		return false
	}
	for _, c := range name {
		if !strings.Contains(validBytes, string(c)) {
			return false
		}
	}
	return true
}

func MakeSession(name string) *Session {
	// We assume ValidSessionName was called
	spawn := strings.HasPrefix(name, "+")
	if spawn {
		name = name[1:]
	}
	s := &Session{
		Name:  name,
		path:  filepath.Join(user.HomeDir, rcdir, "@"+name),
		spawn: spawn,
		tilde: byte('P' & 0x1f),
	}
	os.MkdirAll(s.path, 0700)
	return s
}

func (s *Session) Remove() {
	if s.path != "" {
		os.RemoveAll(s.path)
	}
}

func (s *Session) readfile(n string) (string, error) {
	data, err := ioutil.ReadFile(filepath.Join(s.path, n))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Session) writefile(name, data string) error {
	return ioutil.WriteFile(filepath.Join(s.path, name), ([]byte)(data), 0600)
}

func (s *Session) Pid() (int, bool) {
	data, err := s.readfile("pid")
	if err != nil {
		return 0, false
	}
	if pid, err := strconv.Atoi(data); err == nil {
		return pid, true
	}
	return 0, false
}

func (s *Session) Title() string {
	if data, err := s.readfile("title"); err == nil {
		return data
	}
	return ""
}

func (s *Session) Addr() string {
	if data, err := s.readfile("addr"); err == nil {
		return data
	}
	return ""
}

func (s *Session) TTYSize() string {
	if data, err := s.readfile("ttysize"); err == nil {
		return data
	}
	return ""
}

func (s *Session) DebugPath() string {
	return filepath.Join(s.path, "debug")
}

func (s *Session) SetPid(pid int) error {
	return s.writefile("pid", strconv.Itoa(pid))
}

func (s *Session) SetTitle(title string) error {
	return s.writefile("title", title)
}

func (s *Session) SetAddr(addr string) error {
	// s.addr = addr
	return s.writefile("addr", addr)
}

func (s *Session) SetTTYSize(rows, cols int) error {
	// s.addr = addr
	err := s.writefile("ttysize", fmt.Sprintf("(%dx%d)", cols, rows))
	log.Infof("Setting size to (%dx%d): %v", cols, rows, err)
	return err
}

func (s *Session) Ping() bool {
	pid, ok := s.Pid()
	return ok && syscall.Kill(pid, 0) == nil
}

func (s *Session) Check() bool {
	if !s.Ping() {
		return false
	}
	msg, err := s.Command(askCountMessage, countMessage)
	if err != nil {
		return false
	}
	if cnt, err := strconv.Atoi(msg); err == nil {
		s.cnt = cnt
		return true
	}
	return false
}

func (s *Session) Dial() (net.Conn, error) {
	start := time.Now()
	for {
		if s.Addr() != "" {
			break
		}
		if time.Now().Sub(start) > time.Second*5 {
			return nil, fmt.Errorf("session %s not found", s.Name)
		}
		time.Sleep(time.Second / 10)
	}
	addr, err := net.ResolveTCPAddr("tcp", s.Addr())
	if err != nil {
		return nil, err
	}
	log.Infof("Dialing %s @ %v", s.Name, addr)
	return net.DialTCP("tcp", nil, addr)
}

func (s *Session) Listen() (net.Listener, error) {
	addr := &net.TCPAddr{
		IP: net.IPv4(127, 0, 0, 1),
	}
	conn, err := net.ListenTCP("tcp", addr)
	if err != nil {
		s.Exitf("server: %v", err)
	}
	if err := s.SetAddr(conn.Addr().String()); err != nil {
		s.Remove()
		conn.Close()
		return nil, err
	}
	if err := s.SetPid(os.Getpid()); err != nil {
		s.Remove()
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (s *Session) Command(req, resp messageKind) (string, error) {
	client, err := s.Dial()
	if err != nil {
		log.Infof("Dialing %s %v", s.Name, err)
		s.Remove()
		if strings.Contains(err.Error(), "connect: connection refused") {
			return "", removedErr
		}
		return "", err
	}
	defer func() {
		checkClose(client)
	}()

	w := NewMessengerWriter(client)
	w.Sendf(req, "")
	ch := make(chan []byte, 2)

	r := NewMessengerReader(client, func(kind messageKind, data []byte) {
		switch kind {
		case startMessage:
			w.Sendf(req, "")
		case resp:
			ch <- data
		}
	})

	go func() {
		var buf [256]byte
		for {
			if _, err := r.Read(buf[:]); err != nil {
				log.Infof("Done reading from %s", s.Name)
				return
			}
		}
	}()
	select {
	case <-time.After(time.Second * 5):
		return "", fmt.Errorf("Session %s timed out", s.Name)
	case msg := <-ch:
		return string(msg), nil
	}
}

func (s *Session) PS() string {
	pid, ok := s.Pid()
	if ok {
		return PS(pid)
	}
	return ""
}

func isPipe() bool {
	st, _ := os.Stdin.Stat()
	return (uint32(st.Mode()) & uint32(os.ModeNamedPipe)) != 0
}

func (s *Session) MakeRaw() (err error) {
	if isPipe() {
		return nil
	}
	if s.ostate != nil {
		s.MakeCooked()
		s.Exitf("Calling MakeRaw on a raw session.")
	}
	s.ostate, err = terminal.MakeRaw(0)
	return err
}

func (s *Session) MakeCooked() (err error) {
	if isPipe() {
		return nil
	}
	if s.ostate == nil {
		return nil
	}
	if err := terminal.Restore(0, s.ostate); err != nil {
		return err
	}
	s.ostate = nil
	return nil
}

// A client session must always call Exit or Exitf to make sure the
// terminal is left in a cooked state.  It is safe to call this from
// the server as MakeCooked will do nothing.

func (s *Session) Exit(code int) {
	s.MakeCooked()
	log.DepthErrorf(1, "exit code %d", code)
	exit(code)
}

func (s *Session) Exitf(format string, v ...interface{}) {
	s.MakeCooked()
	log.DepthErrorf(1, format, v...)
	printf(format, v...)
	exit(1)
}
