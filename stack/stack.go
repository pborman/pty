package stack

import (
	"fmt"
	"io"
	"runtime"
	"strings"
)

// A Logger has the standard logging Info method.
type Logger interface {
	Info(...interface{})
}

type logger struct {
	w io.Writer
}

func (l *logger) Info(args ...interface{}) {
	fmt.Fprintln(l.w, args...)
}

// NewLogger returns a simple logger which writes to w.
func NewLogger(w io.Writer) Logger {
	return &logger{w: w}
}

// Dump dumps stack frames i through n to the logger log.
func Dump(log Logger, i, n int) {
	if n <= i {
		n = i + 1
	}
	for i <= n {
		line := Frame(i)
		i++
		if line == "" {
			return
		}
		log.Info(line)
	}
}

// DumpString returns stack frames i through n as a string.
func DumpString(i, n int) string {
	var b strings.Builder
	Dump(NewLogger(&b), i+1, n+1)
	return b.String()
}

// Frame returns the stack frame i.
func Frame(i int) string {
	pc, file, line, ok := runtime.Caller(i)
	if !ok {
		return ""
	}
	fname := ""
	if f := runtime.FuncForPC(pc); f != nil {
		fname = f.Name()
		fname = fname[strings.LastIndex(fname, ".")+1:]
	}
	return fmt.Sprintf("%s:%d %s()", file, line, fname)
}

// Location returns the file:linenumber of stack frame i.
func Location(i int) string {
	_, file, line, ok := runtime.Caller(i)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s:%d", file, line)
}
