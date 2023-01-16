package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/ssh/terminal"
)

func main() {
	var buf [4096]byte

	fmt.Printf("Connected:\n")
	terminal.MakeRaw(0)
	for {
		n, err := os.Stdin.Read(buf[:])
		if n > 0 {
			if string(buf[:4]) == "exit" {
				return
			}
			os.Stdout.Write(buf[:n])
		}
		if n == 0 || err != nil {
			fmt.Fprintf(os.Stdout, "End of input\n")
			return
		}
	}

}
