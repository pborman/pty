package mutex

import (
	"fmt"
	"sync"
	"time"

	"github.com/pborman/pty/log"
)

var CheckStdin = func() {}

type Mutex struct {
	msg      string
	mu       sync.Mutex
}

type waiter *int

// New returns a new named mutex.
func New(msg string) *Mutex {
	m := &Mutex{
		msg: msg,
	}
	m.logf("created")
	return m
}

func (m *Mutex) SetWait(d time.Duration) {
}

func (m *Mutex) logf(format string, args ...interface{}) {
	format = fmt.Sprintf("%p:%s: %s", m, m.msg, format)
	log.Outputf(3, "M", format, args...)
}

// Lock locks a mutex and returns the function to unlock the mutex.
func (m *Mutex) Lock(msg string) func() {
	CheckStdin()
	m.mu.Lock()
	CheckStdin()

	return func() {
		CheckStdin()
		m.mu.Unlock()
		CheckStdin()
	}
}
