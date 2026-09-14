package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/pborman/ansi"
)

func main() {
	// UTF8 keeps multibyte runes intact (bytes 0x80-0x9f are not treated
	// as C1 controls), matching the old unicode-safe Reader.
	r := ansi.Decoder{UTF8: true}.NewReader(os.Stdin)
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
		// A plain-text run has an empty Type; its bytes are in Code.
		if s.Type == "" && len(s.Code) > 0 {
			data := []byte(s.Code)
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
