package log

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	Dir      = os.Getenv("HOME") + "/.pty/log"
	Duration = time.Hour
	Age      = time.Hour * 6
)

type Logger struct {
	mu   sync.Mutex
	fd   io.WriteCloser
	path string
	last string
	quit bool
	done chan struct{}
}

var logger *Logger

func Standard() *Logger { return logger }

func Me() string {
	var me string
	for i := 1; i < 15; i++ {
		pc, file, _, ok := runtime.Caller(i)
		if !ok {
			break
		}
		if strings.Contains(file, "github.com/pborman/pty") {
			if f := runtime.FuncForPC(pc); f != nil {
				me = f.Name()
				me = me[strings.LastIndex(me, ".")+1:]
			}
		}
	}
	return me
}

// Init initializes the logging system.
func Init(path string) error {
	if logger != nil {
		return logger.repath(path)
	}
	var err error
	logger, err = NewLogger(path)
	return err
}

func (log *Logger) repath(path string) error {
	if !strings.HasPrefix(path, "/") {
		path = filepath.Join(Dir, path)
	}
	log.mu.Lock()
	log.path = path
	log.mu.Unlock()
	return log.open()
}

func NewLogger(path string) (*Logger, error) {
	if !strings.HasPrefix(path, "/") {
		path = filepath.Join(Dir, path)
	}
	log := &Logger{
		path: path,
		done: make(chan struct{}),
	}
	if err := log.open(); err != nil {
		return nil, err
	}
	go func() {
		for _ = range time.Tick(Duration) {
			if log.quit {
				log.fd.Close()
				close(log.done)
				return
			}
			if err := log.open(); err != nil {
				log.Errorf("failed to rotate log: %v", err)
			}
			log.mu.Lock()
			path = filepath.Dir(log.path)
			log.mu.Unlock()
			log.prune(filepath.Dir(path))
		}
	}()
	return log, nil
}

func TakeStderr() {
	if f, ok := logger.fd.(*os.File); ok {
		Infof("Taking over stderr")
		syscall.Dup2(int(f.Fd()), 2)
		fmt.Fprintf(os.Stderr, "Took over stderr")
	} else {
		Infof("Failed to take over stderr")
	}
}

const pformat = "20060102.150405"

// prune walks through the logging directory removing every file that
// looks like a logging file and is over Age.
func (log *Logger) prune(dir string) {
	fd, err := os.Open(dir)
	if err != nil {
		log.Errorf("pruning: %v", err)
		return
	}
	names, err := fd.Readdirnames(-1)
	fd.Close()
	if err != nil {
		log.Errorf("pruning: %v", err)
		return
	}
	expire := time.Now().Add(-Age)
	last := filepath.Base(log.last)
	for _, name := range names {
		p := strings.LastIndex(name, ".")
		if name == last {
			continue
		}
		if p < 0 {
			continue
		}
		if _, err := strconv.Atoi(name[p+1:]); err != nil {
			continue
		}
		t := strings.LastIndex(name[:p], "-")
		if t < 0 {
			continue
		}
		ts, err := time.ParseInLocation(pformat, name[t+1:p], time.Local)
		if err != nil {
			continue
		}
		if !ts.Before(expire) {
			continue
		}
		log.Infof("removing old log %s", name)
		os.Remove(filepath.Join(dir, name))
	}
}

func (l *Logger) open() error {
	os.MkdirAll(filepath.Dir(l.path), 0700)
	path := fmt.Sprintf("%s-%s.%d", l.path, time.Now().Format(pformat), os.Getpid())
	fd, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return err
	}

	// Need to do this outside of the lock, but l.fd will only
	// be nil the very first time we are called and then we know
	// there is no race.
	if l.fd != nil {
		l.Infof("switching to log %s", path)
	}
	l.mu.Lock()
	if l.fd != nil {
		l.fd.Close()
	}
	l.fd = fd
	last := l.last
	l.last = path
	l.mu.Unlock()
	if last != "" {
		l.Infof("switched from log %s", last)
	}
	return nil
}

func last2(path string) string {
	n := len(path)
	for i := 0; i < 2; i++ {
		x := strings.LastIndex(path[:n], "/")
		if x < 0 {
			return path
		}
		n = x
	}
	return path[n+1:]
}

func (l *Logger) Info(v ...interface{}) {
	l.Outputf(2, "I", "%s", fmt.Sprint(v...))
}
func (l *Logger) Outputf(depth int, prefix string, format string, v ...interface{}) {
	_, file, line, ok := runtime.Caller(depth + 1)
	if !ok {
		file = "???"
		line = 0
	} else {
		file = last2(file)
	}
	msg := fmt.Sprintf(format, v...)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	msg = fmt.Sprintf("%s%s %s: %s:%d] %s\n", prefix, time.Now().Format("15:04:05.000"), Me(), file, line, msg)
	if l == nil {
		fmt.Fprint(os.Stderr, msg)
		return
	}
	l.mu.Lock()
	fmt.Fprintf(l.fd, msg)
	l.mu.Unlock()
}

func Errorf(format string, v ...interface{})            { logger.Outputf(1, "E", format, v...) }
func Warnf(format string, v ...interface{})             { logger.Outputf(1, "W", format, v...) }
func Infof(format string, v ...interface{})             { logger.Outputf(1, "I", format, v...) }
func Outputf(n int, p, format string, v ...interface{}) { logger.Outputf(n, p, format, v...) }

func DepthErrorf(depth int, format string, v ...interface{}) {
	logger.Outputf(depth+1, "E", format, v...)
}
func DepthWarnf(depth int, format string, v ...interface{}) {
	logger.Outputf(depth+1, "W", format, v...)
}
func DepthInfof(depth int, format string, v ...interface{}) {
	logger.Outputf(depth+1, "I", format, v...)
}

func (log *Logger) Errorf(format string, v ...interface{}) { log.Outputf(1, "E", format, v...) }
func (log *Logger) Warnf(format string, v ...interface{})  { log.Outputf(1, "W", format, v...) }
func (log *Logger) Infof(format string, v ...interface{})  { log.Outputf(1, "I", format, v...) }

func (log *Logger) DumpGoroutines() {
	if p := pprof.Lookup("goroutine"); p != nil {
		var b bytes.Buffer
		p.WriteTo(&b, 2)
		log.Errorf("Dumping current goroutines")
		log.mu.Lock()
		log.fd.Write([]byte{'\n'})
		log.fd.Write(CleanStack(b.Bytes()))
		log.fd.Write([]byte{'\n'})
		log.mu.Unlock()
	} else {
		log.Errorf("failed to lookup goroutine profile")
	}
}

func (log *Logger) DumpStack() {
	n := 15
	for i := 1; i <= n; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		fname := ""
		if f := runtime.FuncForPC(pc); f != nil {
			fname = f.Name()
			fname = fname[strings.LastIndex(fname, ".")+1:]
		}
		log.Infof("%s:%d %s()", file, line, fname)
	}

}

func DumpGoroutines() { logger.DumpGoroutines() }
func DumpStack()      { logger.DumpStack() }
