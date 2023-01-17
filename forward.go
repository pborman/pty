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

type forwarder struct {
	lconn  net.Listener
	mu     *mutex.Mutex
	remote string
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
	f.remote = remote
	return nil
}

func NewForwarder(name, socket string) error {
	os.Remove(socket)
	conn, err := ListenSocket(socket)
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

func (f *forwarder) session(c net.Conn, remote string) error {
	if remote == "" {
		return fmt.Errorf("empty remote name")
	}

	rc, err := DialSocket(remote)
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
