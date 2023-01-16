package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
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
	"golang.org/x/crypto/ssh/terminal"
)

var (
	ostate *terminal.State
	tilde  = byte('P' & 0x1f)
)

func init() {
	mutex.CheckStdin = checkStdin
}

var pprofFd *os.File

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
	defer func() {
		if p := recover(); p != nil {
			log.Errorf("Panic: %v", p)
			log.DumpGoroutines()
			panic(p)
		}
	}()
	if fi.Mode()&0777 != 0700 {
		exitf("pty dir has mode %v, want %v", fi.Mode(), os.FileMode(os.ModeDir|0700))
	}
	if err := ReadConfig(); err != nil {
		exitf("reading configuration file: %v", err)
	}

	internal := getopt.StringLong("internal", 0, "", "internal only flag")
	internalDebug := getopt.StringLong("internal_debug", 0, "", "internal only flag")

	echar := getopt.StringLong("escape", 'e', "^P", "escape character")
	newSession := getopt.StringLong("new", 0, "", "create new session named NAME", "NAME")
	debugFlag := getopt.BoolLong("debug", 0, "debug mode, leave server in foreground")
	debugServer := getopt.BoolLong("debug_server", 0, "enable server debugging")
	detach := getopt.BoolLong("detach", 0, "create and detach new shell, do not connect")
	list := getopt.BoolLong("list", 0, "just list existing sessions")
	getopt.Parse()

	if *list {
		sis := GetSessions()
		fmt.Printf("Found %d sessions:\n", len(sis))
		for _, si := range sis {
			fmt.Printf("  %s (%d)\n", si.Name, si.Count)
		}
		return
	}

	if os.Getenv("_PTY_SHELL") != "" {
		exitf("cannot run pty within a shell spawned by pty")
	}

	// If internal is set then we are being called from spawSession.
	if *internal != "" {
		log.Init("S" + strings.TrimPrefix(filepath.Base(*internal), "session"))
		log.TakeStderr()
		runServer(*internal, *internalDebug)
		return
	}
	checkStdin()

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

	var ok bool
	tilde, ok = parseEscapeChar(*echar)
	if !ok {
		exitf("invalid escape character: %q", *echar)
	}

	checkStdin()
	var session string
	var isNew bool
	switch {
	case *newSession != "":
	checkStdin()
		session = SessionPath(*newSession)
	checkStdin()
		if _, _, err := CheckSession(session); err == nil {
	checkStdin()
			exitf("session name already in use")
		}
	checkStdin()
		isNew = true
	checkStdin()
	case len(args) == 0:
	checkStdin()
		session, isNew, err = SelectSession()
	checkStdin()
		switch err {
		case nil:
	checkStdin()
		case io.EOF:
	checkStdin()
			exit(1)
		default:
	checkStdin()
			exitf("selecting session: %v", err)
		}
	checkStdin()
		if session == "" {
	checkStdin()
			exit(42)
		}
	case len(args) > 0:
	checkStdin()
		session = SessionPath(args[0])
	checkStdin()
		cnt, pid, err := CheckSession(session)
	checkStdin()
		if err != nil {
	checkStdin()
			exitf("no such session %s", args[0])
		}
		if cnt == 0 {
	checkStdin()
			break
		}
	checkStdin()
		fmt.Print(ps)
	checkStdin()
		ok, err := readYesNo("Session has %d clients.\n%sContinue? [Y/n] ", cnt, PS(pid))
	checkStdin()
		if err != nil {
	checkStdin()
			exitf("reading: %v", err)
	checkStdin()
		}
	checkStdin()
		if !ok {
	checkStdin()
			return
		}
	}

	checkStdin()
	log.Init("C" + strings.TrimPrefix(filepath.Base(session), "session"))
	checkStdin()
	log.TakeStderr()
	checkStdin()

	if isNew {
	checkStdin()
		var debugFile string
	checkStdin()
		if *debugServer {
	checkStdin()
			debugFile = session + debugSuffix
	checkStdin()
		}
	checkStdin()
log.Infof("SPAWN NEW SERVER")
		spawnServer(session, debugFile, *debugFlag)
	checkStdin()
		if *detach {
	checkStdin()
			return
	checkStdin()
		}
	checkStdin()
		// Give the new shell a chance to start up.
	checkStdin()
	checkStdin()
	}

	// Here on down is the pty client.
	checkStdin()
	checkStdin()
	amClient = true // make checkStdin do something
	checkStdin()

	checkStdin()
	c, err := net.DialUnix("unix", nil, &net.UnixAddr{
		Name: session,
		Net:  "unix",
	})
	checkStdin()

	if err != nil {
	checkStdin()
		exitf("dialing session: %v", err)
	}

	checkStdin()
	defer exit(0) // main should not return, this is a failsafe
	checkStdin()
	myname, _ := ttyname.Fileno(0)
	checkStdin()
	if myname == "" {
		myname = "unknown"
	}
	checkStdin()
	displayMotd()
	checkStdin()
	checkStdin()
	checkStdin()
	ostate, err = terminal.MakeRaw(0)
	checkStdin()
	checkStdin()
	if err != nil {
	checkStdin()
		exitf("stty: %v\n", err)
	}
	checkStdin()
	if !isNew {
	checkStdin()
		fmt.Printf("Connected to session %s\r\n", SessionName(session))
	}
	checkStdin()
	if tilde != 0 {
	checkStdin()
		fmt.Printf("Escape character is %s\r\n", printEscape(tilde))
	}
	checkStdin()

	w := NewMessengerWriter(c)
	checkStdin()
	checkStdin()
	ready := make(chan struct{})
	checkStdin()
	/*go*/ func() {
	checkStdin()
		defer func() {
	checkStdin()
			log.Errorf("MessengerReader is done")
			if p := recover(); p != nil {
				log.Errorf("Panic: %v", p)
				log.DumpGoroutines()
				panic(p)
			}
		}()
		checkStdin()
		mr := NewMessengerReader(c, func(kind messageKind, data []byte) {
			checkStdin()
			clientCommand(w, kind, data, ready)
			checkStdin()
		})
		checkStdin()
		var buf [1024]byte
		checkStdin()
		var err error
		checkStdin()
		var n int
		checkStdin()
		for err == nil {
			checkStdin()
			n, err = mr.Read(buf[:])
			checkStdin()
			if n > 0 {
				checkStdin()
				if _, err := os.Stdout.Write(buf[:n]); err != nil {
					log.Errorf("Writing to stdout: %v", err)
				}
				checkStdin()
				// tee.Write(buf[:n])
				checkStdin()
			}
			checkStdin()
		}
		checkStdin()
		if err != nil && err != io.EOF {
			exitf("client exit: %v", err)
		}
		exit(0)
	}()

	checkStdin()
	// Below is the code that reads from stdin and writes to the server.
		checkStdin()
	watchSigwinch(w)
		checkStdin()
	w.Sendf(ttynameMessage, "%s", myname)
		checkStdin()
	var buf [32768]byte
	state := 0
	<-ready
	ecnt := 0
rcnt := 0
	for {
		checkStdin()
		log.Infof("%d Reading from stdin", rcnt)
		rcnt++
		checkStdin()
		n, rerr := os.Stdin.Read(buf[:])
		if rerr != nil {
			log.Infof("%v", err)
		}
		checkStdin()
		if n == 0 {
			log.Errorf("Stdin(%d) returned %v", os.Stdin.Fd(), rerr)
			stdin, err := os.Open("/dev/stdin")
			if err != nil {
				exitf("%v", err)
			}
			if err := syscall.Dup2(int(stdin.Fd()), 0); err != nil {
				exitf("%v", err)
			}
			n, rerr = os.Stdin.Read(buf[:])
			if n == 0 {
				exitf(" after reopening: %v", rerr)
			}
		}
		checkStdin()
		log.Infof("Read %d", n)
		checkStdin()
		var cmd byte
		if tilde != 0 {
			checkStdin()
		Loop:
			// Look tilde followed by . or :
			for _, c := range buf[:n] {
				checkStdin()
				switch state {
				case 0:
					switch c {
					case tilde:
						state = 1
					}
				case 1:
					switch c {
					case '.', ':':
						cmd = c
						state = 2
						break Loop
					case tilde:
						// we should probably strip one of the two tilde's.
						n = 0
						state = 0
						checkStdin()
						w.Write([]byte{tilde})
						checkStdin()
						break Loop
					default:
						state = 0
					}
				}
			}
			checkStdin()
			if state >= 1 {
				n -= state
			}
		}
		checkStdin()
		if n > 0 {
			checkStdin()
			_, err2 := w.Write(buf[:n])
			checkStdin()
			if err == nil {
				err = err2
			}
		}
		checkStdin()
		if cmd != 0 {
			log.Infof("request command %q", cmd)
		}
		checkStdin()
		switch cmd {
		case 0:
			continue
		case tilde:
			checkStdin()
			if _, err := os.Stdout.Write([]byte{tilde}); err != nil {
log.Infof("%v", err)
			}
			checkStdin()
		case '.':
			checkStdin()
			if _, err := os.Stdout.Write([]byte("\r\n")); err != nil {
log.Infof("%v", err)
	}
			checkStdin()
			exit(0)
		case ':':
			checkStdin()
			terminal.Restore(0, ostate)
			checkStdin()
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
			if len(args) > 0 {
				command(false, w, args...)
			}
			checkStdin()
			ostate, err = terminal.MakeRaw(0)
			checkStdin()
			if err != nil {
				exitf("stty: %v\n", err)
			}
			command(true, w, args...)
		}
		checkStdin()
		log.Infof("finished command %q", cmd)
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
		checkStdin()
	}

	if !strings.Contains(err.Error(), "broken pipe") {
		exitf("%v", err)
	}
	exit(0)
}

var amClient = false
var csMu sync.Mutex
var csWhen = map[string]time.Time{}
var csLine = map[string]string{}

func checkStdin() {
log.Outputf(2, "S", "Checking stdin")
	if !amClient {
		return
	}
	csMu.Lock()
	now := time.Now()
	var sb syscall.Stat_t

	myLine := "????"
	if _, file, line, ok := runtime.Caller(1); ok {
		myLine = fmt.Sprintf("%s:%d", file, line)
	}

	me := log.Me()

	if err := syscall.Fstat(0, &sb); err != nil {
		oldWhen := csWhen[me]
		oldLine := csLine[me]
		log.Errorf("\nstdin failed %v between %v and %v (%v)\n%s Previous: %s\n%s Current : %s\n",
			err,
			oldWhen.Format("15:04:05.000"),
			now.Format("15:04:05.000"),
			now.Sub(oldWhen), me, oldLine, me, myLine)
		fmt.Printf("\r\nstdin failed %v between %v and %v (%v)\r\n%s Previous: %s\r\n%s Current : %s\r\n",
			err,
			oldWhen.Format("15:04:05.000"),
			now.Format("15:04:05.000"),
			now.Sub(oldWhen), me,  oldLine, me, myLine)
for k, v := range csLine {
	if k != me {
	fmt.Printf("other goroutine: %s: %s\r\n", k, v)
	log.Errorf("other goroutine: %s: %s: %s\r\n", k, csWhen[k].Format("15:04:05.000"), v)
	}
}
		log.DumpStack()
		csMu.Unlock()
		select {}
	}
	csLine[me] = myLine
	csWhen[me] = now
	csMu.Unlock()
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
func clientCommand(w *MessengerWriter, kind messageKind, data []byte, ready chan struct{}) {
	checkStdin()
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
		checkStdin()
		os.Stdout.Write(data)
		checkStdin()
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
		checkStdin()
		rows, cols, err := pty.Getsize(os.Stdin)
		checkStdin()
		if err == nil {
			w.Send(ttysizeMessage, encodeSize(rows, cols))
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
return len(buf), nil // we are not using tee so we don't want mutex log messages
	unlock := t.mu.Lock("Write")
	w := t.w
	unlock()
	if w == nil {
		return len(buf), nil
	}
	checkStdin()
	return w.Write(buf)
}

func (t *teeer) Open(path string) {
log.Infof("Opening tee")
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

func command(raw bool, w *MessengerWriter, args ...string) {
	if len(args) == 0 {
		return
	}
	switch args[0] {
	case "help":
		if raw {
			return
		}
		fmt.Printf("Commands:\n")
		fmt.Printf("  dump   - dump stack\n")
		fmt.Printf("  excl   - detach all other clients\n")
		fmt.Printf("  env    - display environment variables\n")
		fmt.Printf("  list   - list all clients\n")
		fmt.Printf("  save   - save buffer to FILE\n")
		fmt.Printf("  setenv - list all clients\n")
		fmt.Printf("  ssh    - forward SSH_AUTH_SOCK\n")
		fmt.Printf("  tee    - tee all future output to FILE (- to close)\n")
		fmt.Printf("  ps     - display processes on this pty\n")
	case "dump":
		if raw {
			w.Send(dumpMessage, nil)
		} else {
			log.DumpGoroutines()
		}
	case "list":
		if raw {
			w.Send(listMessage, nil)
		}
	case "ps":
		if raw {
			return
		}
		checkStdin()
		os.Stdout.Write(ps(w))
	case "excl":
		if raw {
			w.Send(exclusiveMessage, nil)
		}
	case "getenv", "env":
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
	case "save":
		if !raw && len(args) != 2 {
			fmt.Printf("usage: save FILENAME\n")
			return
		}
		if raw && len(args) == 2 {
			w.Send(saveMessage, []byte(args[1]))
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
	case "escapes":
		if raw {
			return
		}
		if len(args) != 2 {
			fmt.Printf("usage: escapes [alt|normal]\n")
			return
		}
		w.Send(escapeMessage, []byte(args[1]))
	default:
		if !raw {
			fmt.Printf("unknown command: %s\n", args[0])
		}
	}
}

func watchSigwinch(w *MessengerWriter) error {
	return nil
	checkStdin()
	rows, cols, err := pty.Getsize(os.Stdin)
	checkStdin()
	if err == nil {
		w.Send(ttysizeMessage, encodeSize(rows, cols))
	}
	if err != nil {
		return nil
	}
panic("cannot get here")
	go func() {
		defer func() {
			log.Errorf("watchSigwinch is done")
			if p := recover(); p != nil {
				log.Errorf("Panic: %v", p)
				log.DumpGoroutines()
				panic(p)
			}
		}()
		ch := make(chan os.Signal, 2)
		signal.Notify(ch, syscall.SIGWINCH)
		for range ch {
			checkStdin()
			rows, cols, err := pty.Getsize(os.Stdin)
			checkStdin()
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
