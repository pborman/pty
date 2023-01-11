package main

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

const sockname = "/tmp/unix.test"

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: unix {server|client}")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "server":
		server()
	case "client":
		client()
	}
}

func server() {
	os.Remove(sockname)
	conn, err := net.ListenUnix("unix", &net.UnixAddr{
		Name: sockname,
		Net:  "unix",
	})
	f, err := conn.File()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fd := int(f.Fd())
	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_PASSCRED, 1)
	fmt.Printf("Listening on %d\n", fd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for {
		c, err := conn.Accept()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		var oob [256]byte
		var buf [256]byte
		n, oobn, _, _, err := c.(*net.UnixConn).ReadMsgUnix(buf[:], oob[:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		scm, err := syscall.ParseSocketControlMessage(oob[:oobn])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		for _, m := range scm {
			cred, err := syscall.ParseUnixCredentials(&m)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return
			}
			fmt.Printf("cred: %+v\n", cred)
		}
		fmt.Printf("buf: %q\n", buf[:n])
		c.Close()
	}
}

func client() {
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{
		Name: sockname,
		Net:  "unix",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	conn.Write([]byte("hello world\n"))
	var buf [256]byte
	fmt.Println(conn.Read(buf[:]))
}
