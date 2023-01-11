package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/pborman/pty/ansi"
)

func main() {
	r := ansi.NewReader(os.Stdin)
	w := bufio.NewWriter(os.Stdout)
	var sawCR bool
	for {
		s, err := r.Next()
		if err != nil {
			if err != io.EOF {
				fmt.Fprintln(os.Stderr, err)
			}
			break
		}
		if s.Code == "" && len(s.Text) > 0 {
			data := []byte(s.Text)
			if data[0] == '\n' && sawCR {
				data = data[1:]
				sawCR = false
			}
			if len(data) > 0 {
				sawCR = data[len(data)-1] == '\r'
			}
			for len(data) > 0 {
				x := bytes.Index(data, []byte{'\r'})
				if x < 0 {
					w.Write(data)
					break
				}
				data[x] = '\n'
				w.Write(data[:x+1])
				data = data[x+1:]
				if len(data) > 0 && data[0] == '\n' {
					data = data[1:]
				}
			}
		}
	}
	w.Flush()
}
