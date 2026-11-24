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
	"sync/atomic"
	"syscall"
	"time"
)

var (
	Dir      = os.Getenv("HOME") + "/.pty/log"
	Duration = time.Hour
	Age      = time.Hour * 6
)

type Logger struct {
	mu        sync.Mutex
	fd        io.WriteCloser
	path      string
	last      string
	quit      chan struct{} // closed by Close to stop rotation
	closeOnce sync.Once
	done      chan struct{} // closed when rotation has stopped
	verbose   atomic.Bool
}

var logger *Logger

func Standard() *Logger { return logger }
func SetVerbose(v bool) { logger.SetVerbose(v) }

// Me returns the the function name of the deepest stack frame that is part of
// the pty program.
func Me() string {
	var me string
	for i := 1; i < 15; i++ {
		pc, file, _, ok := runtime.Caller(i)
		if !ok {
			break
		}
		if strings.Contains(file, "/pty/") {
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
		quit: make(chan struct{}),
		done: make(chan struct{}),
	}
	if err := log.open(); err != nil {
		return nil, err
	}
	// Read Duration here, not in the goroutine, so a later change to
	// it affects only loggers created after the change.
	go log.rotate(time.NewTicker(Duration))
	return log, nil
}

// rotate starts a new log file, and prunes old ones, every time t ticks,
// until Close is called.
func (log *Logger) rotate(t *time.Ticker) {
	defer close(log.done)
	defer t.Stop()
	for {
		select {
		case <-log.quit:
			log.mu.Lock()
			log.fd.Close()
			log.mu.Unlock()
			return
		case <-t.C:
		}
		if err := log.open(); err != nil {
			log.Errorf("failed to rotate log: %v", err)
		}
		log.mu.Lock()
		dir := filepath.Dir(log.path)
		log.mu.Unlock()
		log.prune(dir)
	}
}

// Close stops log rotation and closes the current log file.  Messages logged
// after Close are discarded.  Close may be called more than once.
func (log *Logger) Close() error {
	log.closeOnce.Do(func() { close(log.quit) })
	<-log.done
	return nil
}

func (log *Logger) SetVerbose(v bool) {
	log.verbose.Store(v)
}

func TakeStderr() {
	if f, ok := logger.fd.(*os.File); ok {
		Infof("Taking over stderr")
		syscall.Dup2(int(f.Fd()), 2)
		fmt.Fprintf(os.Stderr, "Took over stderr\n")
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
	log.mu.Lock()
	last := filepath.Base(log.last)
	log.mu.Unlock()
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

func (log *Logger) open() error {
	os.MkdirAll(filepath.Dir(log.path), 0700)
	path := fmt.Sprintf("%s-%s.%d", log.path, time.Now().Format(pformat), os.Getpid())
	fd, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return err
	}

	// Announce the switch in the old file.  Infof takes log.mu, so check
	// for an old file under the lock but log outside it.
	log.mu.Lock()
	switching := log.fd != nil
	log.mu.Unlock()
	if switching {
		log.Infof("switching to log %s", path)
	}
	log.mu.Lock()
	if log.fd != nil {
		log.fd.Close()
	}
	log.fd = fd
	last := log.last
	log.last = path
	log.mu.Unlock()
	if last != "" {
		log.Infof("switched from log %s", last)
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

func (log *Logger) Outputf(depth int, prefix string, format string, v ...interface{}) {
	if prefix == "D" && !log.verbose.Load() {
		return
	}
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
	if log == nil {
		fmt.Fprint(os.Stderr, msg)
		return
	}
	// msg is already formatted; writing it with Fprintf would mangle
	// any % it contains.
	log.mu.Lock()
	io.WriteString(log.fd, msg)
	log.mu.Unlock()
}

func Errorf(format string, v ...interface{})            { logger.Outputf(1, "E", format, v...) }
func Warnf(format string, v ...interface{})             { logger.Outputf(1, "W", format, v...) }
func Infof(format string, v ...interface{})             { logger.Outputf(1, "I", format, v...) }
func Debugf(format string, v ...interface{})            { logger.Outputf(1, "D", format, v...) }
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
func (log *Logger) Debugf(format string, v ...interface{}) { log.Outputf(1, "D", format, v...) }

func (log *Logger) DumpGoroutines() {
	if !log.verbose.Load() {
		return
	}
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
	if !log.verbose.Load() {
		return
	}
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
