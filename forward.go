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
	"net"
	"sync"

	"github.com/pborman/pty/log"
	"github.com/pborman/pty/mutex"
)

type forwarder struct {
	lconn  net.Listener
	mu     *mutex.Mutex
	remote *Session
}

var (
	forwardersMu sync.Mutex
	forwarders   = map[string]*forwarder{}
)

func SetForwarder(name, remote string) error {
	forwardersMu.Lock()
	f := forwarders[name]
	forwardersMu.Unlock()
	if f == nil {
		return fmt.Errorf("no such socket: %s", name)
	}
	defer f.mu.Lock("SetForwarder")()
	f.remote = MakeSession(remote, "")
	return nil
}

func NewForwarder(name, socket string) error {
	s := MakeSession(socket, "")
	s.Remove()
	conn, err := s.Listen()
	if err != nil {
		return err
	}
	f := &forwarder{
		mu:    mutex.New("Forwarder: " + name),
		lconn: conn,
	}
	forwardersMu.Lock()
	forwarders[name] = f
	forwardersMu.Unlock()
	go f.server()
	return nil
}

func (f *forwarder) server() {
	for {
		c, err := f.lconn.Accept()
		if err != nil {
			// send some sort of message?
			log.Infof("Accept: %v", err)
			return
		}
		go func() {
			unlock := f.mu.Lock("server")
			remote := f.remote
			unlock()
			f.session(c, remote)
			checkClose(c)
		}()
	}
}

func (f *forwarder) session(c net.Conn, s *Session) error {
	rc, err := s.Dial()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		io.Copy(c, rc)
		wg.Done()
	}()
	go func() {
		io.Copy(rc, c)
		wg.Done()
	}()
	wg.Wait()
	checkClose(rc)
	return nil
}
