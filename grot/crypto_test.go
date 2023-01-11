package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"
)

func runData(t *testing.T, tag string, c *Channel, lengths []int) {
	first := tag == "client"
	n := 0
	failed := false
	for i, count := range lengths {
		sample := genData(count)
		if first {
			if _, err := c.Write(sample); err != nil {
				t.Errorf("%s: %d: write(%d) -> %v", tag, i, count, err)
				return
			}
		} else {
			buf := make([]byte, count)
			if err := c.ReadFull(buf); err != nil {
				t.Errorf("%s: %d: readfull(%d) -> %v", tag, i, count, err)
				return
			}
			if !failed && !bytes.Equal(buf, sample) {
				failed = true
				t.Errorf("%s:%d: after %d: got(%d) %.16x, want %.16x", tag, i, n, count, buf, sample)
			}
		}
		first = !first
		n += count
	}
}

func TestEncryptedSessions(t *testing.T) {
	socket := fmt.Sprintf("/tmp/pty.test.%d", os.Getpid())
	defer os.Remove(socket)
	sdone := make(chan bool, 1)
	cdone := make(chan bool, 1)

	d1 := genData(1024)
	d2 := genData(1024)
	if !bytes.Equal(d1, d2) {
		t.Fatalf("genData is not deterministic")
	}

	// Try 2048 buffers (1024 in each direction)
	// All lengths in the range of [1..1024] will be tried.
	// All Lengths are in the range of [1..4096].

	lengths := make([]int, 2048)
	r := rand.New(rand.NewSource(17))

	// First fill [0..1024) with the 1024 required values
	for i := 0; i < 1024; i++ {
		lengths[i] = i + 1
	}

	// Then fill [1024..2048) with deterministic random values
	for i := 1024; i < 2048; i++ {
		lengths[i] = 1 + r.Intn(4096)
	}

	go func() {
		err := EncryptedUnixServer(socket, func(c *Channel, err error) {
			defer func() { sdone <- true }()
			if err != nil {
				t.Errorf("accept: %v", err)
				return
			}
			runData(t, "server", c, lengths)
		})
		if err != nil {
			t.Errorf("server: %v", err)
		}
		sdone <- true
	}()
	time.Sleep(time.Second / 10)
	go func() {
		defer func() { cdone <- true }()
		c, err := EncryptedUnixDial(socket)
		if err != nil {
			t.Errorf("EncryptedUnixDial: %v", err)
			return
		}
		runData(t, "client", c, lengths)
	}()
	<-cdone
	<-sdone
}

// genData returns a byte slice of count bytes with deterministic random
// contents.
func genData(count int) []byte {
	data := make([]byte, count)
	r := rand.New(rand.NewSource(int64(count)))
	if _, err := r.Read(data); err != nil {
		panic("rand read failed")
	}
	return data
}
