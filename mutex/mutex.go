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

// Package mutex provides a mutex lock that can be traced when debugging.
// Internally the mutex contains a sync.Mutex to handle the actual locking,
// blocking, and releasing. Every Mutex must be initialized with New and
// provided a name.  Lock is provided with who wants to lock the lock.  Since
// the correspoding unlock *must* use the sane name provided to Lock, the method
// to unlock the lock is returned.
//
// Examples
//
//	m := mutex.New("my lock")
//
//	unlock := m.Lock("fred")
//	...
//	unlock()
//
//	// Defer the unlock routine
//	defer m.Lock("bob")()
//
// If the environment variable __MUTEX_DEBUG has the value of "true" when the
// first lock is initialized, debugging and tracing is turned on.  When
// __MUTEX_DEBUG is true then the Dump routine will print out a list of all held
// locks, who is holding it, and who is blocked on it.  Debugging is heavy
// weight.  Each time a lock is taken an internal sync.Mutex is locked and
// unlocked twice.  When the lock is released the internal sync.Mutex is also
// locked and unlocked.  The function runtime.Caller is also called for each
// lock and unlock when debugging is enabled.
//
// When __MUTEX_DEBUG is not set, or set to "false" then debugging is not
// enabled and this package turns into a thin wrapper around sync.Mutex.
//
// BUG:  Currently this package use the github.com/pborman/pty/log logging
// package when debugging.  I would like to remove the dependencies however
// there is a race condition during initialization between the first call to New
// and when a logger could be registered.
package mutex

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/pborman/pty/log"
)

// A Mutex is a mutex.
type Mutex struct {
	name    string
	mu      sync.Mutex
	imu     sync.Mutex
	owner   string
	index   int
	waiting map[string]struct{}
}

var (
	mu        sync.Mutex
	list      []*Mutex // A list of all the mutexes ever created.
	index     int      // to make all mutex names unique
	underTest = func(string) {}
	debug     = false
	once      sync.Once
	logger    = log.Outputf
)

// New returns a new named mutex.
func New(name string) *Mutex {
	once.Do(func() {
		// The very first time we are called we determine if we are
		// debugging or not.
		switch strings.ToLower(os.Getenv("__MUTEX_DEBUG")) {
		case "t", "true", "yes", "1":
			debug = true
		case "f", "false", "no", "0":
			debug = false
		}
	})
	if !debug {
		return &Mutex{}
	}
	m := &Mutex{
		name:    location(index, name),
		waiting: map[string]struct{}{},
	}
	index++
	mu.Lock()
	list = append(list, m)
	mu.Unlock()
	return m
}

// Lock waits until it aquires the mutex log m and then returns the function
// that will unlock m.
func (m *Mutex) Lock(who string) func() {
	// When debug is not set we are just a thin wrapper around sync.Mutex.
	if !debug {
		m.mu.Lock()
		return m.mu.Unlock
	}

	who = location(-1, who)
	m.logf("%s waiting for mutex", who)

	// Mark us waiting on the lock
	m.imu.Lock()
	{
		m.waiting[who] = struct{}{}
		m.imu.Unlock()
	}

	m.mu.Lock()

	// Mark us as owner of the lock and no longer waiting.
	m.imu.Lock()
	{
		delete(m.waiting, who)
		m.owner = who
	}
	m.imu.Unlock()
	m.logf("%s acquired", who)

	return func() {
		m.logf("%s releasing mutex", who)
		var owner string
		m.imu.Lock()
		{
			owner = who
			m.owner = ""
		}
		m.imu.Unlock()

		if owner != who {
			who := fmt.Sprintf("Mutex %s owned by %s and %s", m.name, owner, who)
			if underTest != nil {
				underTest(who)
			} else {
				panic(who)
			}
		}
		m.mu.Unlock()
	}
}

// Dump dumps the state of all non-idle muticies to w if the environment variable
// __MUTEX_DEBUG is set to "true".  An environment variable is used as it is the
// only reasonable way to turn debugging on prior to the first call to New.
func Dump(w io.Writer) {
	if !debug {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, m := range list {
		// Don't lock m, we are looking for mutecies that are currently
		// locked with potentially waiting callers.
		if m.owner != "" {
			fmt.Fprintf(w, "mutex %s is locked by %s\n", m.name, m.owner)
		} else if len(m.waiting) != 0 {
			// This really should never happen.
			fmt.Fprintf(w, "mutex %s is idle\n", m.name)
		}
		for name := range m.waiting {
			fmt.Fprintf(w, "   %s waiting\n", name)
		}
	}
}

func (m *Mutex) logf(format string, args ...interface{}) {
	if !debug {
		return
	}
	format = fmt.Sprintf("%s (%s)", format, m.name)
	logger(3, "M", format, args...)
}

func location(x int, msg string) string {
	_, file, line, _ := runtime.Caller(2)
	file = filepath.Base(file)
	if x < 0 {
		return fmt.Sprintf("<%s:%d %s>", file, line, msg)
	}
	return fmt.Sprintf("<[%d] %s:%d %s>", x, file, line, msg)
}
