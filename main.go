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

	"github.com/creack/pty"
	"github.com/pborman/options"
	"github.com/pborman/pty/log"
	"github.com/pborman/pty/mutex"
	"github.com/pborman/pty/parse"
	ttyname "github.com/pborman/pty/tty"
)

var pprofFd *os.File

// opts holds the command line options.
var opts = struct {
	Internal      string `getopt:"--internal       internal only flag"`
	InternalDebug string `getopt:"--internal_debug internal only flag"`
	Escape        string `getopt:"--escape -e=CHAR escape character"`
	ID            string `getopt:"--id=ID          originating ID (TERM_SESSION_ID)"`
	New           string `getopt:"--new=NAME       create new session named NAME"`
	Debug         bool   `getopt:"--debug          debug mode, leave server in foreground"`
	DebugServer   bool   `getopt:"--debug_server   enable server debugging"`
	Detach        bool   `getopt:"--detach         create and detach new shell, do not connect"`
	Dead          bool   `getopt:"--dead           list the dead sessions"`
	Clean         bool   `getopt:"--clean          cleanup dead sessions"`
	List          bool   `getopt:"--list           just list existing sessions"`
	MList         bool   `getopt:"--mlist          machine readable list of existing sessions"`
	Auto          bool   `getopt:"--auto           automatically attach to matching session"`
	Create        bool   `getopt:"--create -c      create session if not existing"`
	Yes           bool   `getopt:"--yes -y         automatically answer yes to 'Continue?'"`
	Grok          bool   `getopt:"--grok           grok input mapping (DEL -> \\b)"`
	Hexdump       string `getopt:"--hexdump=FILE   hexdump terminal I/O to FILE"`
	Verbose       bool   `getopt:"--verbose -v     turn on verbose debug"`
}{
	Escape: "^P",
}

func countBools(vs ...bool) int {
	cnt := 0
	for _, v := range vs {
		if v {
			cnt++
		}
	}
	return cnt
}

func main() {
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
	SelectLoginShell(config.Shells)
	if LoginShell == "" {
		exitf("no available shells")
	}

	args := options.RegisterAndParse(&opts)
	log.SetVerbose(opts.Verbose)

	switch countBools(opts.List, opts.MList, opts.Dead, opts.Clean, opts.Auto, opts.Create) {
	case 0:
	case 1:
	default:
		exitf("At most one of --create, --auto, --list, --mlist, --dead, or --clean can be specified")
	}
	if opts.List {
		sis := GetSessions()
		fmt.Printf("Found %d sessions:\n", len(sis))
		for _, si := range sis {
			fmt.Printf("  %s (%d) %s\n", si.Name, si.cnt, si.Title())
		}
		return
	}

	if opts.MList {
		for _, si := range GetSessions() {
			fmt.Printf("%d:%s\n", si.cnt, si.Name)
		}
		return
	}

	if opts.Dead {
		for _, si := range GetDeadSessions() {
			fmt.Printf("%s\n", si.Name)
		}
		return
	}

	if opts.Clean {
		for _, si := range GetDeadSessions() {
			fmt.Printf("Removing %s\n", si.Name)
			os.RemoveAll(si.Path())
		}
		return
	}

	if os.Getenv("_PTY_SHELL") != "" {
		exitf("cannot run pty within a shell spawned by pty")
	}

	// If internal is set then we are being called from spawSession.
	if opts.Internal != "" {
		if !ValidSessionName(strings.TrimPrefix(opts.Internal, "/")) {
			exitf("invalid session name %q", opts.Internal)
		}

		session := MakeSession(opts.Internal, opts.ID)
		log.Init(session.path + "/log/server")
		log.TakeStderr()
		log.SetVerbose(opts.Verbose)
		session.run(opts.InternalDebug)
		return
	}

	switch len(args) {
	case 0:
	case 1:
		if opts.New != "" {
			options.PrintUsage(os.Stderr)
			os.Exit(1)
		}
	default:
		options.PrintUsage(os.Stderr)
		os.Exit(1)
	}

	tilde, ok := parseEscapeChar(opts.Escape)
	if !ok {
		exitf("invalid escape character: %q", opts.Escape)
	}

	var session *Session
	switch {
	case opts.New != "":
		if !ValidSessionName(opts.New) {
			exitf("invalid session name %q", opts.New)
		}

		session = MakeSession(opts.New, opts.ID)
		if session.Check() {
			exitf("session name already in use")
		}
	case len(args) == 0:
		session, err = SelectSession(opts.ID)
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
		session = MakeSession(args[0], opts.ID)

		if !session.Check() {
			if opts.Create {
				session = MakeSession(args[0], opts.ID)
				if session.Check() {
					exitf("session name already in use")
				}
				break
			}
			exitf("no such session %s", args[0])
		}
		if session.cnt == 0 || opts.Yes {
			break
		}
		ok, err := readYesNo("Session has %d client%s.\nContinue? [y/n] ", session.cnt, splur(session.cnt))
		if err != nil {
			exitf("reading: %v", err)
		}
		if !ok {
			return
		}
	}

	log.Init(session.path + "/log/client")
	log.SetVerbose(opts.Verbose)
	log.TakeStderr()
	session.tilde = tilde

	if !session.Ping() {
		var debugFile string
		if opts.DebugServer {
			debugFile = session.DebugPath()
		}

		// Spawn withdraws the address of the server that is gone, so
		// Dial below waits for the new server to publish its own,
		// which it does once it is accepting connections.
		session.Spawn(debugFile, opts.Debug)
		if opts.Detach {
			return
		}
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

	if opts.Hexdump != "" {
		if err := hexdumps.Open(opts.Hexdump); err != nil {
			exitf("hexdump: %v", err)
		}
	}

	// A previous tenant of this terminal may have left it non-blocking.
	// Session.Exit clears it again on the way out, so we do not hand the
	// condition on to whatever runs next.
	if clearNonblock() {
		log.Warnf("terminal was left in non-blocking mode, cleared")
	}

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
	var server peerVersion // the server's protocol version
	go func() {
		// read from the server and write to stdout
		write := func(buf []byte) {
			hexdumps.Dump("OUT", buf)
			if _, err := os.Stdout.Write(buf); err != nil {
				log.Errorf("Writing to stdout: %v", err)
			}
			tee.Write(buf)
		}
		if err := readServer(c, w, ready, &server, session, write); err != io.EOF {
			exitf("client exit: %v", err)
		}
		exit(0)
	}()

	// Below is the code that reads from stdin and writes to the server.
	watchSigwinch(w, session)
	w.Sendf(ttynameMessage, "%d:%s", os.Getpid(), myname)
	const readSize = 32768
	// Room for a held-back partial clear sequence released by the filter.
	var buf [readSize + len(clearIntro)]byte
	var cf clearFilter
	sendClear := func(screen []byte) {
		hexdumps.Dump("CLR", screen)
		sendClear(w, &server, screen)
	}
	esc := escapeKey{key: session.tilde}
	echo := func(b []byte) { os.Stdout.Write(b) }
	// Nothing is read from stdin until the server has sent startMessage.
	// By then the server's version is known: it is sent first, and
	// messages are handled in order, so sendClear can tell whether the
	// server handles clearMessage.
	<-ready
	for {
		n, err := os.Stdin.Read(buf[:readSize])
		hexdumps.Dump("IN", buf[:n])
		n = copy(buf[:], cf.Filter(buf[:n], sendClear))

		// A read may hold commands, each followed by more input.
		for data := buf[:n]; ; {
			input, cmd, rest := esc.Scan(data)
			if len(input) > 0 {
				if opts.Grok {
					input = bytes.ReplaceAll(input, []byte{0x7f}, []byte{0x08})
				}
				hexdumps.Dump("SND", input)
				_, err2 := w.Write(input)
				if err == nil {
					err = err2
				}
			}
			if err != nil {
				log.Infof("Exiting on error %v", err)
				exit(0)
			}
			if cmd == 0 {
				break
			}
			log.Infof("request command %q", cmd)
			if cmd == '.' {
				if _, err := os.Stdout.Write([]byte("\r\n")); err != nil {
					log.Infof("%v", err)
				}
				exit(0)
			}
			// cmd is ':'.  What was typed after it in the same
			// read starts the command line.
			session.MakeCooked()
			fmt.Printf("\nCommand: ")
			line, more, err := lineFrom(rest, echo, readline)
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
			data = more
		}
	}
}

var (
	ackerMu sync.Mutex
	ackers  = map[[16]byte]chan struct{}{}

	// psChan is where the answer to the pending ps request goes, or nil
	// if none is pending.  It is buffered so that clientCommand never
	// blocks on it.
	psMu   sync.Mutex
	psChan chan []byte
)

func ps(w *MessengerWriter) []byte {
	ch := make(chan []byte, 1)
	psMu.Lock()
	psChan = ch
	psMu.Unlock()
	defer func() {
		psMu.Lock()
		if psChan == ch {
			psChan = nil
		}
		psMu.Unlock()
	}()
	w.Send(psMessage, nil)
	select {
	case data := <-ch:
		return data
	case <-time.After(15 * time.Second):
		return nil
	}
}

// psAnswer passes data, the answer to a ps request, to ps.  An answer that
// arrives after ps has given up is dropped.
func psAnswer(data []byte) {
	psMu.Lock()
	ch := psChan
	psChan = nil
	psMu.Unlock()
	if ch != nil {
		ch <- data
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

// readServer reads from the server on c until the read fails, passing
// terminal output to write and messages to clientCommand.  It returns the
// error that ended it, io.EOF if the server went away.
func readServer(c io.Reader, w *MessengerWriter, ready chan struct{}, server *peerVersion, s *Session, write func([]byte)) error {
	mr := NewMessengerReader(c, func(kind messageKind, data []byte) {
		clientCommand(w, kind, data, ready, s, server)
	})
	var buf [1024]byte
	for {
		n, err := mr.Read(buf[:])
		if n > 0 {
			write(buf[:n])
		}
		if err != nil {
			return err
		}
	}
}

// sendClear sends screen, from a clear sequence the terminal sent (see
// clear.go), to the server in a clearMessage, if the server handles them.
// An older server would answer with an error, so it is not told; its record
// of the screen is then not replaced.
func sendClear(w *MessengerWriter, server *peerVersion, screen []byte) {
	if !server.Supports(versionClear) {
		log.Infof("server does not handle clearMessage; clear not sent")
		return
	}
	w.Send(clearMessage, screen)
}

// clientCommand handles command recevied from the server.  Server is what
// the client knows of the server's protocol version.
func clientCommand(w *MessengerWriter, kind messageKind, data []byte, ready chan struct{}, s *Session, server *peerVersion) {
	log.Infof("Received command %v", kind)
	switch kind {
	case versionMessage:
		v := parseVersion(data)
		log.Infof("server protocol version %d", v)
		// Reply only once, and only now: a server that never
		// sends its version would not understand ours.
		if server.set(v) {
			w.Send(versionMessage, versionPayload())
		}
	case pingMessage:
		w.Send(ackMessage, data)
	case psMessage:
		psAnswer(data)
	case ackMessage:
		ackerMu.Lock()
		defer ackerMu.Unlock()
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
		// The server sends its version before startMessage, so
		// if it has not sent one it never will.
		server.setDefault()
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
		case '"', '\\', '$', '`':
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
			log.ForceDumpGoroutines()
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
