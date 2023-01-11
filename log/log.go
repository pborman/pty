package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
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

func (l *Logger) Outputf(depth int, prefix string, format string, v ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
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
	fmt.Fprintf(l.fd, "%s%s %s:%d] %s\n", prefix, time.Now().Format("150304.000"), file, line, msg)
}

func Errorf(format string, v ...interface{}) { logger.Outputf(1, "E", format, v...) }
func Warnf(format string, v ...interface{})  { logger.Outputf(1, "W", format, v...) }
func Infof(format string, v ...interface{})  { logger.Outputf(1, "I", format, v...) }

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

func (log *Logger) LogStack() {
	if p := pprof.Lookup("goroutine"); p != nil {
		log.Errorf("Dumping current goroutines")
		log.mu.Lock()
		log.fd.Write([]byte{'\n'})
		p.WriteTo(log.fd, 2)
		log.fd.Write([]byte{'\n'})
		log.mu.Unlock()
	} else {
		log.Errorf("failed to lookup goroutine profile")
	}
}
func LogStack() { logger.LogStack() }
