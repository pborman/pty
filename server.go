package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/pborman/pty/log"
)

func shell(socket string, debug bool) {
	s := NewShell(socket)
	if err := s.Start(debug); err != nil {
		exitf("start: %v\n", err)
	}
	go func() {
		s.Wait()
		os.Exit(0)
	}()

	conn, err := net.ListenUnix("unix", &net.UnixAddr{
		Name: socket,
		Net:  "unix",
	})
	if err != nil {
		exitf("server: %v", err)
	}

	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGABRT, syscall.SIGBUS, syscall.SIGQUIT, syscall.SIGSEGV)
	go func() {
		for s := range ch {
			log.Errorf("signal %v", s)
			log.LogStack()
			switch s {
			case syscall.SIGABRT, syscall.SIGBUS, syscall.SIGSEGV:
				exitf("exiting on signal %d", s)
			}
		}
	}()

	for {
		c, err := conn.Accept()
		if err != nil {
			exitf("server: %v", err)
		}
		log.Infof("accepted new connection")
		go func() {
			attach(c, s)
			c.Close()
		}()
	}
}

func attach(c net.Conn, s *Shell) {
	// Attach forwards the shells output to c.
	mw := NewMessengerWriter(c)
	client := NewClient(mw)
	cnt := s.Attach(client)
	defer func() { go s.Detach(client) }()

	ech := make(chan error, 1)
	go func() {
		r := NewMessengerReader(c, func(kind messageKind, msg []byte) {
			log.Infof("received message %q", kind)
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
				s.mu.Lock()
				var clients []*Client
				for c := range s.clients {
					if c != client {
						clients = append(clients, c)
					}
				}
				s.mu.Unlock()
				for _, oc := range clients {
					s.Detach(oc)
					oc.Output([]byte(fmt.Sprintf("\r\nDetached by client %s\r\n", client.Name())))
					oc.Close()
				}
			case askCountMessage:
				mw.Sendf(countMessage, "%d:%d", cnt, os.Getpid())
			case pingMessage:
				mw.Send(ackMessage, msg)
			case ttynameMessage:
				client.SetName(string(msg))
			case listMessage:
				s.List(client)
			case ttysizeMessage:
				s.Take(client, false)
				if len(msg) != 4 {
					mw.Sendf(serverMessage, "ERROR: SCREEN MSG IS %d BYTES, need 4\r\n", len(msg))
					return
				}
				rows, cols := decodeSize(msg)
				s.mu.Lock()
				defer s.mu.Unlock()
				if rows == s.rows && cols == s.cols {
					return
				}
				s.rows = rows
				s.cols = cols
				if err := s.Setsize(rows, cols); err != nil {
					mw.Sendf(serverMessage, "ERROR: SETSIZE: %v\r\n", err)
				}
			case saveMessage:
				var err error
				s.mu.Lock()
				if s.eb.inalt {
					err = ioutil.WriteFile(string(msg), s.eb.alt, 0600)
				} else {
					err = ioutil.WriteFile(string(msg), s.eb.normal, 0600)
				}
				s.mu.Unlock()
				if err != nil {
					mw.Sendf(serverMessage, "ERROR: saving screen: %v\n", err)
				} else {
					mw.Sendf(serverMessage, "screen saved to %s\r\n", msg)
				}
			case escapeMessage:
				s.mu.Lock()
				s.eb.sendEscapes(mw, strings.ToLower(string(msg)) == "alt")
				s.mu.Unlock()
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
	c.Close()
}

// spawnServer spans a server process.
func spawnServer(session, debugFile string, foreground bool) {
	if foreground {
		runServer(session, debugFile)
		return
	}
	// We prepend session with a "+" to indicate we
	// want our child to fork another child and then
	// exit.
	args := []string{"--internal", "+" + session}
	if debugFile != "" {
		args = append(args, "--internal_debug", session+debugSuffix)
	}

	cmd := exec.Command(os.Args[0], args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{}

	if err := cmd.Start(); err != nil {
		exitf("starting subprocess: %v", err)
	}
	fmt.Printf("Started session %s\n", SessionName(session))
	go func() {
		err := cmd.Wait()
		if err != nil {
			fmt.Printf("internal shell exited: %v", err)
		}
	}()
}

func runServer(session, debugFile string) {
	if session[0] != '+' {
		if debugFile != "" {
			debugInit(debugFile)
		}
		shell(session, false)
		return
	}

	args := []string{"--internal", session[1:]}
	if debugFile != "" {
		args = append(args, "--internal_debug", debugFile)
	}
	cmd := exec.Command(os.Args[0], args...)
	if err := cmd.Start(); err != nil {
		log.Errorf("rexec failed: %v", err)
		exitf("rexec failed: %v\n", err)
	}
}
