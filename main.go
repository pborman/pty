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
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kr/pty"
	"github.com/pborman/getopt"
	"github.com/pborman/pty/log"
	"github.com/pborman/pty/mutex"
	"github.com/pborman/pty/parse"
	ttyname "github.com/pborman/pty/tty"
)

var pprofFd *os.File
var autoAttach *bool

func main() {
	os.Setenv("GORACE", "log_path=/tmp/cloud_race")
	log.Init("pty")
	dir := filepath.Join(user.HomeDir, rcdir)
	os.Mkdir(dir, 0700)
	os.Chmod(dir, 0700)
	fi, err := os.Stat(dir)
	if err != nil {
		exitf("no pty dir: %v", err)
	}

	if fi.Mode()&0777 != 0700 {
		exitf("pty dir has mode %v, want %v", fi.Mode(), os.FileMode(os.ModeDir|0700))
	}
	if err := ReadConfig(); err != nil {
		exitf("reading configuration file: %v", err)
	}

	internal := getopt.StringLong("internal", 0, "", "internal only flag")
	internalDebug := getopt.StringLong("internal_debug", 0, "", "internal only flag")

	echar := getopt.StringLong("escape", 'e', "^P", "escape character")
	sessionID := getopt.StringLong("id", 0, "", "originating ID (TERM_SESSION_ID)")
	newSession := getopt.StringLong("new", 0, "", "create new session named NAME", "NAME")
	debugFlag := getopt.BoolLong("debug", 0, "debug mode, leave server in foreground")
	debugServer := getopt.BoolLong("debug_server", 0, "enable server debugging")
	detach := getopt.BoolLong("detach", 0, "create and detach new shell, do not connect")
	list := getopt.BoolLong("list", 0, "just list existing sessions")
	autoAttach = getopt.BoolLong("auto", 0, "automatically attach to matching session")
	createSession := getopt.BoolLong("create", 'c', "creatre session if not existing")
	getopt.Parse()

	if *list {
		sis := GetSessions()
		fmt.Printf("Found %d sessions:\n", len(sis))
		for _, si := range sis {
			fmt.Printf("  %s (%d) %s\n", si.Name, si.cnt, si.Title())
		}
		return
	}

	if os.Getenv("_PTY_SHELL") != "" {
		exitf("cannot run pty within a shell spawned by pty")
	}

	// If internal is set then we are being called from spawSession.
	if *internal != "" {
		session := MakeSession(*internal, *sessionID)
		log.Init(session.path + "/log/server")
		log.TakeStderr()
		session.run(*internalDebug)
		return
	}

	args := getopt.Args()
	switch len(args) {
	case 0:
	case 1:
		if *newSession != "" {
			getopt.PrintUsage(os.Stderr)
			os.Exit(1)
		}
	default:
		getopt.PrintUsage(os.Stderr)
		os.Exit(1)
	}

	tilde, ok := parseEscapeChar(*echar)
	if !ok {
		exitf("invalid escape character: %q", *echar)
	}

	var session *Session
	switch {
	case *newSession != "":
		session = MakeSession(*newSession, *sessionID)
		if session.Check() {
			exitf("session name already in use")
		}
	case len(args) == 0:
		session, err = SelectSession(*sessionID)
		switch err {
		case nil:
		case io.EOF:
			exit(1)
		default:
			exitf("selecting session: %v", err)
		}
		if session == nil {
			exit(42)
		}
	case len(args) > 0:
		if !ValidSessionName(args[0]) {
			exitf("invalid session name %q", args[0])
		}
		session = MakeSession(args[0], *sessionID)

		if !session.Check() {
			if *createSession {
				session = MakeSession(args[0], *sessionID)
				if session.Check() {
					exitf("session name already in use")
				}
				break
			}
			exitf("no such session %s", args[0])
		}
		if session.cnt == 0 {
			break
		}
		ok, err := readYesNo("Session has %d client%s.\nContinue? [Y/n] ", session.cnt, splur(session.cnt))
		if err != nil {
			exitf("reading: %v", err)
		}
		if !ok {
			return
		}
	}

	log.Init(session.path + "/log/client")
	log.TakeStderr()
	session.tilde = tilde

	if !session.Ping() {
		var debugFile string
		if *debugServer {
			debugFile = session.DebugPath()
		}

		session.Spawn(debugFile, *debugFlag)
		if *detach {
			return
		}
		// Give the new shell a chance to start up.
		time.Sleep(time.Second / 2)
	}

	// Here on down is the pty client.
	c, err := session.Dial()

	if err != nil {
		exitf("dialing session: %v", err)
	}

	defer exit(0) // main should not return, this is a failsafe
	myname, _ := ttyname.Fileno(0)
	if myname == "" {
		myname = "unknown"
	}
	displayMotd()

	if err := session.MakeRaw(); err != nil {
		exitf("stty: %v\n", err)
	}

	// Here on down we need to use session.exit
	exit := session.Exit
	exitf := session.Exitf

	if !session.started {
		fmt.Printf("Connected to session %s\r\n", session.Name)
	}
	if session.tilde != 0 {
		fmt.Printf("Escape character is %s\r\n", printEscape(session.tilde))
	}

	w := NewMessengerWriter(c)
	ready := make(chan struct{})
	go func() {
		// read from the server and write to stdout
		mr := NewMessengerReader(c, func(kind messageKind, data []byte) {
			clientCommand(w, kind, data, ready, session)
		})
		var buf [1024]byte
		var err error
		var n int
		var sendSSHb = ([]byte)(sendSSH)

		write := func(buf []byte) {
			if len(buf) == 0 {
				return
			}
			if _, err := os.Stdout.Write(buf); err != nil {
				log.Errorf("Writing to stdout: %v", err)
			}
			tee.Write(buf)
		}
		for err == nil {
			n, err = mr.Read(buf[:])
			wbuf := buf[:n]

			// We assume our magic escape sequence always
			// comes in a single message
			if x := bytes.Index(wbuf, sendSSHb); x >= 0 {
				write(wbuf[:x])
				command(true, session, w, "ssh")
				wbuf = wbuf[x+len(sendSSH):]
			}
			write(wbuf)
		}
		if err != nil && err != io.EOF {
			exitf("client exit: %v", err)
		}
		exit(0)
	}()

	// Below is the code that reads from stdin and writes to the server.
	watchSigwinch(w, session)
	w.Sendf(ttynameMessage, "%d:%s", os.Getpid(), myname)
	var buf [32768]byte
	state := 0
	<-ready
	ecnt := 0
	rcnt := 0
	for {
		rcnt++
		n, rerr := os.Stdin.Read(buf[:])

		var cmd byte
		if session.tilde != 0 {
		Loop:
			// Look tilde followed by . or :
			for _, c := range buf[:n] {
				switch state {
				case 0:
					switch c {
					case session.tilde:
						state = 1
					}
				case 1:
					switch c {
					case '.', ':':
						cmd = c
						state = 2
						break Loop
					case session.tilde:
						// we should probably strip one of the two tilde's.
						n = 0
						state = 0
						w.Write([]byte{session.tilde})
						break Loop
					default:
						state = 0
					}
				}
			}
			if state >= 1 {
				n -= state
			}
		}
		if n > 0 {
			_, err2 := w.Write(buf[:n])
			if err == nil {
				err = err2
			}
		}
		if cmd != 0 {
			log.Infof("request command %q", cmd)
		}
		switch cmd {
		case 0:
			continue
		case session.tilde:
			if _, err := os.Stdout.Write([]byte{session.tilde}); err != nil {
				log.Infof("%v", err)
			}
		case '.':
			if _, err := os.Stdout.Write([]byte("\r\n")); err != nil {
				log.Infof("%v", err)
			}
			exit(0)
		case ':':
			session.MakeCooked()
			fmt.Printf("\nCommand: ")
			line, err := readline()
			if err != nil {
				exitf("readline: %v\n", err)
			}
			args, err := parse.Line(line)
			if err != nil {
				log.Warnf("parse %q: %v", line, err)
				fmt.Printf("%v\n", err)
			}
			command(false, session, w, args...)
			if err := session.MakeRaw(); err != nil {
				exitf("stty: %v\n", err)
			}
			command(true, session, w, args...)
		}

		state = 0
		if rerr != nil {
			log.Errorf("client read from stdin(%d): %v", os.Stdin.Fd(), rerr)
			ecnt++
			if ecnt > 10 {
				break
			}
		} else {
			ecnt = 0
		}
	}

	if !strings.Contains(err.Error(), "broken pipe") {
		exitf("%v", err)
	}
	exit(0)
}

var (
	ackerMu sync.Mutex
	ackers  = map[[16]byte]chan struct{}{}
	psChan  chan []byte
)

func ps(w *MessengerWriter) []byte {
	psChan = make(chan []byte)
	w.Send(psMessage, nil)
	select {
	case data := <-psChan:
		return data
	case <-time.After(15 * time.Second):
		return nil
	}
}

func ping(w *MessengerWriter) error {
	var data [16]byte
	rand.Read(data[:])
	ch := make(chan struct{})
	ackerMu.Lock()
	ackers[data] = ch
	ackerMu.Unlock()
	w.Send(pingMessage, data[:])
	select {
	case <-ch:
		return nil
	case <-time.After(time.Second * 15):
		return fmt.Errorf("ping timed out")
	}
}

// clientCommand handles command recevied from the server.
func clientCommand(w *MessengerWriter, kind messageKind, data []byte, ready chan struct{}, s *Session) {
	log.Infof("Received command %v", kind)
	switch kind {
	case pingMessage:
		w.Send(ackMessage, data)
	case psMessage:
		if psChan != nil {
			psChan <- data
			close(psChan)
		}
	case ackMessage:
		ackerMu.Unlock()
		var key [16]byte
		copy(key[:], data)
		if ch := ackers[key]; ch != nil {
			close(ch)
		}
		delete(ackers, key)
	case serverMessage:
		os.Stdout.Write(data)
	case countMessage:
	case preemptMessage:
		// We could warn the client
	case waitMessage:
		select {
		case <-ready:
		default:
			readline()
		}
	case startMessage:
		select {
		case <-ready:
		default:
			close(ready)
		}
	case primaryMessage:
		rows, cols, err := pty.Getsize(os.Stdin)
		if err == nil {
			w.Send(ttysizeMessage, encodeSize(rows, cols))
			s.SetTTYSize(rows, cols)
		}
		for _, name := range config.Forward {
			value := os.Getenv(name)
			if value != "" {
				s := fmt.Sprintf("%s\000%s", name, value)
				w.Send(forwardMessage, []byte(s))
			}
		}
	default:
		fmt.Printf("Got message type %d: %q\r\n", kind, data)
	}
}

func quoteShell(s string) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, `"`)
	for _, c := range s {
		switch c {
		case '"', '\\', '$':
			fmt.Fprintf(&buf, `\%c`, c)
		default:
			fmt.Fprintf(&buf, `%c`, c)
		}
	}
	fmt.Fprintf(&buf, `"`)
	return buf.String()
}

type teeer struct {
	mu   *mutex.Mutex
	w    *os.File
	path string
}

var tee = teeer{
	mu: mutex.New("teeer"),
}

func (t *teeer) Write(buf []byte) (int, error) {
	unlock := t.mu.Lock("Write")
	w := t.w
	unlock()
	if w == nil {
		return len(buf), nil
	}
	return w.Write(buf)
}

func (t *teeer) Open(path string) {
	if path == "-" {
		unlock := t.mu.Lock("Open1")
		if t.w != nil {
			if err := checkClose(t.w); err != nil {
				fmt.Printf("ERROR CLOSING TEE: %v\r\n", err)
			}
			t.w = nil
		}
		t.path = ""
		unlock()
		return
	}
	unlock := t.mu.Lock("Open2")
	w := t.w
	unlock()
	if w != nil {
		fmt.Printf("ERROR: already teeing to %s\r\n", t.path)
		return
	}
	w, err := os.Create(path)
	if err != nil {
		fmt.Printf("ERROR OPENING TEE: %v\r\n", err)
		return
	}
	unlock = t.mu.Lock("Open3")
	if t.w == nil {
		t.w = w
		t.path = path
	} else {
		fmt.Printf("ERROR: tee created spontainiously?!\r\n")
	}
	unlock()
}

// Command is called with a ^P: command.  It normally is called twice for each
// command.  The first time it is called "raw" will be set to false indicating
// the terminal is still in cooked mode.  The second time "raw" will be set to
// true indicating the terminal is once again in raw mode.
func command(raw bool, session *Session, w *MessengerWriter, args ...string) {
	if len(args) == 0 {
		return
	}
	switch args[0] {
	case "help":
		if raw {
			return
		}
		fmt.Printf("Commands:\n")
		fmt.Printf("  dump    - dump stack\n")
		fmt.Printf("  env     - display environment variables of client\n")
		fmt.Printf("  escapes - display escape sequences in save buffers\n")
		fmt.Printf("  excl    - detach all other clients\n")
		fmt.Printf("  list    - list all clients\n")
		fmt.Printf("  ps      - display processes on this pty\n")
		fmt.Printf("  save    - save buffer to FILE\n")
		fmt.Printf("  setenv  - forward environtment variables\n")
		fmt.Printf("  ssh     - forward SSH_AUTH_SOCK\n")
		fmt.Printf("  tee     - tee all future output to FILE (- to close)\n")
		fmt.Printf("  title   - set the title for this session\n")
	case "dump":
		if raw {
			w.Send(dumpMessage, nil)
		} else {
			log.DumpGoroutines()
		}
	case "env", "getenv":
		if raw {
			return
		}
		args = args[1:]
		if len(args) == 0 {
			for _, name := range os.Environ() {
				var value string
				if x := strings.Index(name, "="); x > 0 {
					value = name[x+1:]
					name = name[:x]
				}
				fmt.Printf("%s=%s\n", name, quoteShell(value))
			}
			return
		}
		for _, name := range args {
			if value, ok := os.LookupEnv(name); ok {
				fmt.Printf("%s=%s\n", name, quoteShell(value))
			} else {
				fmt.Printf("%s not set\n", name)
			}
		}
	case "escapes":
		if raw {
			return
		}
		if len(args) != 2 {
			fmt.Printf("usage: escapes [alt|normal]\n")
			return
		}
		w.Send(escapeMessage, []byte(args[1]))
	case "excl":
		if raw {
			w.Send(exclusiveMessage, nil)
		}
	case "list":
		if raw {
			w.Send(listMessage, nil)
		}
	case "ps":
		if raw {
			return
		}
		os.Stdout.Write(ps(w))
	case "save":
		if !raw && len(args) != 2 {
			fmt.Printf("usage: save FILENAME\n")
			return
		}
		if raw && len(args) == 2 {
			w.Send(saveMessage, []byte(args[1]))
		}
	case "setenv":
		if !raw {
			return
		}
		args = args[1:]
		for _, name := range args {
			if value, ok := os.LookupEnv(name); ok {
				fmt.Fprintf(w, "%s=%s\r", name, quoteShell(value))
			}
		}
	case "ssh":
		if !raw {
			return
		}
		if value, ok := os.LookupEnv("SSH_AUTH_SOCK"); ok {
			fmt.Fprintf(w, "SSH_AUTH_SOCK=%s\r", quoteShell(value))
		}
	case "tee":
		if raw {
			return
		}
		if len(args) != 2 {
			fmt.Printf("usage: tee FILENAME\n")
			return
		}
		tee.Open(args[1])
	case "title":
		if raw {
			return
		}
		if len(args) > 1 {
			session.SetTitle(strings.Join(args[1:], " "))
		}
		fmt.Printf("%s: %s\n", session.Name, session.Title())
	default:
		if !raw {
			fmt.Printf("unknown command: %s\n", args[0])
		}
	}
}

func watchSigwinch(w *MessengerWriter, s *Session) error {
	rows, cols, err := pty.Getsize(os.Stdin)
	if err == nil {
		w.Send(ttysizeMessage, encodeSize(rows, cols))
		s.SetTTYSize(rows, cols)
	}
	if err != nil {
		return nil
	}
	go func() {

		ch := make(chan os.Signal, 2)
		signal.Notify(ch, syscall.SIGWINCH)
		for range ch {
			rows, cols, err := pty.Getsize(os.Stdin)
			if err != nil {
				log.Warnf("sigwinch getsize: %v", err)
				fmt.Fprintf(os.Stderr, "getsize: %v\r\n", err)
			} else {
				log.Infof("sigwinch %d,%d", rows, cols)
				w.Send(ttysizeMessage, encodeSize(rows, cols))
			}
		}
	}()
	return nil
}
