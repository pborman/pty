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
	"os"
	"sync"

	"github.com/pborman/pty/log"
	"github.com/pborman/pty/mutex"
)

// A forwarder lets processes in the shell reach a unix domain socket that
// belongs to whichever client is currently primary, such as that client's
// ssh-agent.  The shell's environment variable (e.g., SSH_AUTH_SOCK) names the
// forwarder's own socket, which never changes; each connection to it is
// proxied to the remote socket most recently reported by a client.  This keeps
// the agent usable across detach and reattach from different ssh logins.
type forwarder struct {
	lconn  net.Listener
	mu     *mutex.Mutex
	remote string // path of the client's socket to forward to
}

var (
	forwardersMu sync.Mutex
	forwarders   = map[string]*forwarder{}
)

// SetForwarder directs future connections to the forwarder for the
// environment variable name to the unix domain socket at remote.
func SetForwarder(name, remote string) error {
	forwardersMu.Lock()
	f := forwarders[name]
	forwardersMu.Unlock()
	if f == nil {
		return fmt.Errorf("no such socket: %s", name)
	}
	defer f.mu.Lock("SetForwarder")()
	f.remote = remote
	return nil
}

// NewForwarder listens on the unix domain socket socket and forwards each
// connection to remote, which SetForwarder may later change.
func NewForwarder(name, socket, remote string) error {
	os.Remove(socket) // left behind by a server that did not exit cleanly
	conn, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	f := &forwarder{
		mu:     mutex.New("Forwarder: " + name),
		lconn:  conn,
		remote: remote,
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
			if err := f.session(c, remote); err != nil {
				log.Warnf("forward to %q: %v", remote, err)
			}
			checkClose(c)
		}()
	}
}

func (f *forwarder) session(c net.Conn, remote string) error {
	if remote == "" {
		return fmt.Errorf("no socket to forward to")
	}
	rc, err := net.Dial("unix", remote)
	if err != nil {
		return err
	}

	done := make(chan struct{}, 1)
	go func() {
		io.Copy(c, rc)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(rc, c)
		done <- struct{}{}
	}()
	<-done
	checkClose(rc)
	return nil
}
