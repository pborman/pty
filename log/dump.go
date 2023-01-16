package log

import (
	"bytes"
	"io"
	"os"
)

var home = append(append([]byte{'\t'}, []byte(os.Getenv("HOME"))...), '/')

var registers = [][]byte{
	([]byte)("rax "),
	([]byte)("rbx "),
	([]byte)("rcx "),
	([]byte)("rdx "),
	([]byte)("rdi "),
	([]byte)("rsi "),
	([]byte)("rbp "),
	([]byte)("rsp "),
	([]byte)("r8 "),
	([]byte)("r9 "),
	([]byte)("r10 "),
	([]byte)("r11 "),
	([]byte)("r12 "),
	([]byte)("r13 "),
	([]byte)("r14 "),
	([]byte)("r15 "),
	([]byte)("rip "),
	([]byte)("rflags "),
	([]byte)("cs "),
	([]byte)("fs "),
	([]byte)("gs "),
}

func isRegister(line []byte) bool {
	for _, r := range registers {
		if bytes.HasPrefix(line, r) {
			return true
		}
	}
	return false
}

func nextLine(buf []byte) (line, rest []byte) {
	if buf == nil {
		panic(io.EOF)
	}
	if x := bytes.IndexByte(buf, '\n'); x >= 0 {
		return buf[:x+1], buf[x+1:]
	}
	return buf, nil
}

// CleanStack prunes the stack to only include local frames.
func CleanStack(buf []byte) []byte {
	pwd, err := os.Getwd()

	if err == nil {
		pwd = "\t" + pwd + "/"
	}
	wd := ([]byte)(pwd)

	defer func() {
		if p := recover(); p != nil {
			if p != io.EOF {
				panic(p)
			}
		}
	}()
	var b bytes.Buffer
	var line, path, newbuf []byte
	var goroutine []byte
	for len(buf) > 0 {
		line, buf = nextLine(buf)
		if bytes.HasPrefix(line, ([]byte)("goroutine ")) {
			if len(newbuf) > 0 {
				b.Write([]byte{'\n'})
				b.Write(goroutine)
				b.Write(newbuf)
			}
			newbuf = nil
			goroutine = line
			continue
		}
		if len(goroutine) == 0 {
			continue
		}
		if len(line) == 0 || line[0] == '\n' {
			continue
		}
		path, buf = nextLine(buf)
		if len(wd) > 0 && bytes.HasPrefix(path, wd) {
			newbuf = append(newbuf, line...)
			newbuf = append(newbuf, '\t')
			newbuf = append(newbuf, path[len(wd):]...)
		} else if bytes.HasPrefix(path, home) {
			newbuf = append(newbuf, line...)
			newbuf = append(newbuf, '\t')
			newbuf = append(newbuf, path[len(home):]...)
		}
	}
	if len(newbuf) > 0 {
		b.Write([]byte{'\n'})
		b.Write(goroutine)
		b.Write(newbuf)
	}
	return b.Bytes()
}
