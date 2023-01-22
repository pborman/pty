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

package mutex

import (
	"fmt"
	"sync"

	"github.com/pborman/pty/log"
)

type Mutex struct {
	msg string
	mu  sync.Mutex
}

const debug = true

type waiter *int

// New returns a new named mutex.
func New(msg string) *Mutex {
	m := &Mutex{
		msg: msg,
	}
	return m
}

func (m *Mutex) logf(format string, args ...interface{}) {
	if !debug {
		return
	}
	format = fmt.Sprintf("%p:%s: %s", m, m.msg, format)
	log.Outputf(3, "M", format, args...)
}

// Lock locks a mutex and returns the function to unlock the mutex.
func (m *Mutex) Lock(msg string) func() {
	m.logf("%s waiting for mutex\n", msg)
	m.mu.Lock()
	m.logf("%s acquired\n", msg)

	return func() {
		m.logf("%s releasing for mutex\n", msg)
		m.mu.Unlock()
	}
}
