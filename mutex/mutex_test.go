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
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func reset(db bool) {
	list = nil
	index = 0
	debug = db
	underTest = func(string) {}
}

func init() {
	// Disable logging
	logger = func(n int, p, format string, v ...interface{}) {}
}

func TestNoDebug(t *testing.T) {
	reset(false)
	m := New("D")

	unlock := m.Lock("1")

	here := false
	go func() {
		defer m.Lock("2")()
		here = true
	}()
	time.Sleep(time.Second / 100)
	if here {
		t.Errorf("double lock")
	}
	unlock()

	time.Sleep(time.Second / 100)
	if !here {
		t.Errorf("lock blocked")
	}
}
func TestSimple(t *testing.T) {
	reset(true)
	m := New("M")
	unlock := m.Lock("L")

	if want, got := 0, len(m.waiting); want != got {
		t.Errorf("Got %d waiting, want %d", want, got)
	}

	fdone := make(chan struct{})
	go func() {
		unlock := m.Lock("B")
		t.Log("B took the lock")
		if want, got := 0, len(m.waiting); want != got {
			t.Errorf("Got %d waiting, want %d", want, got)
		}
		unlock()
		close(fdone)
	}()
	time.Sleep(time.Second / 100)
	tm := time.NewTimer(time.Second)

	go func() {
		<-tm.C
		t.Errorf("timed out")
		Dump(os.Stderr)
	}()

	if want, got := 1, len(m.waiting); want != got {
		t.Errorf("Got %d waiting, want %d", want, got)
	}
	tm.Stop()
	unlock()
	<-fdone // wait for the goroutine to finish
}

func TestDump(t *testing.T) {
	reset(true)
	m1 := New("M1")
	m2 := New("M2")
	m3 := New("M3")
	m3.waiting["ghost"] = struct{}{}

	unlock1 := m1.Lock("B1")
	unlock2 := m2.Lock("B1")

	locked := make(chan bool)   // a goroutine got M1
	unlockit := make(chan bool) // tell a goroutine to unlock M1
	released := make(chan bool) // a goroutine released M1

	// Start up 3 goroutines, all blocked on M1.
	for i := 1; i <= 3; i++ {
		g := fmt.Sprintf("G%d", i)
		go func() {
			unlock := m1.Lock(g)
			locked <- true
			// Main thread is doing a dump at this time.
			<-unlockit
			unlock()
			released <- true
		}()
	}

	// Wait for all three goroutines to block on M1
	for {
		time.Sleep(time.Second / 1000)
		m1.imu.Lock()
		n := len(m1.waiting)
		m1.imu.Unlock()
		if n == 3 {
			break
		}
	}

	// goUnlock tells one goroutine to unlock and waits for it to do so.
	goUnlock := func() {
		unlockit <- true
		<-released
	}

	var lines []string
	var failed bool

	buf := &bytes.Buffer{}

	takeDump := func() {
		failed = false
		buf.Reset()
		Dump(buf)
		s := buf.String()
		if len(s) == 0 {
			lines = []string{}
			return
		}
		s = s[:len(s)-1] // trailing newline
		lines = strings.Split(s, "\n")
	}

	lineHas := func(n int, s string) (ok, exists bool) {
		if n >= len(lines) {
			return false, false
		}
		return strings.Contains(lines[n], s), true
	}

	type test struct {
		n    int
		want string
	}

	testFunc := func(name string, tt test) {
		ok, exists := lineHas(tt.n, tt.want)
		if tt.want == "" {
			if exists {
				failed = true
				t.Errorf("%s: Extra line %d:%s", name, tt.n, lines[tt.n])
			}
		} else if !exists {
			failed = true
			t.Errorf("%s Line %d missing", name, tt.n)
		} else if !ok {
			failed = true
			t.Errorf("%s Line %d missing %s", name, tt.n, tt.want)
		}
	}

	// M1 is locked
	// G1, G2, G3 are blocked on M1
	// M2 is locked
	// M3 is not locked but "ghost" is waiting on it
	// We also know that list is ordered based on the order we created the muticies.
	takeDump()
	for _, tt := range []test{
		{n: 0, want: "M1> is locked by"},
		{n: 1, want: "waiting"},
		{n: 2, want: "waiting"},
		{n: 3, want: "waiting"},
		{n: 4, want: "M2> is locked by"},
		{n: 5, want: "M3> is idle"},
		{n: 6, want: "ghost waiting"},
		{n: 8, want: ""},
	} {
		testFunc("Initial State", tt)
	}

	// Release M2
	unlock2()
	takeDump()
	for _, tt := range []test{
		{n: 0, want: "M1> is locked by"},
		{n: 1, want: "waiting"},
		{n: 2, want: "waiting"},
		{n: 3, want: "waiting"},
		{n: 4, want: "M3> is idle"},
		{n: 5, want: "ghost waiting"},
		{n: 6, want: ""},
	} {
		testFunc("Dropped M2", tt)
	}

	// Remove M3's ghose
	delete(m3.waiting, "ghost")
	takeDump()
	for _, tt := range []test{
		{n: 0, want: "M1> is locked by"},
		{n: 1, want: "waiting"},
		{n: 2, want: "waiting"},
		{n: 3, want: "waiting"},
		{n: 4, want: ""},
	} {
		testFunc("Dropped M3", tt)
	}

	// Unlock m1 and wait for a goroutine to get the lock.
	// Take a dump and then tell the goroutine to unlock and wait for it.
	unlock1()
	<-locked
	takeDump()
	for _, tt := range []test{
		{n: 0, want: "M1> is locked by"},
		{n: 1, want: "waiting"},
		{n: 2, want: "waiting"},
		{n: 3, want: ""},
	} {
		testFunc("1G removed", tt)
	}
	if failed {
		t.Logf("%s", buf.String())
	}

	// Tell the first goroutine to unlock.
	// Wait for another goroutine to grab it.
	goUnlock()
	<-locked
	takeDump()
	for _, tt := range []test{
		{n: 0, want: "M1> is locked by"},
		{n: 1, want: "waiting"},
		{n: 2, want: ""},
	} {
		testFunc("2Gs removed", tt)
	}
	if failed {
		t.Logf("%s", buf.String())
	}

	// Tell the second goroutine to unlock.
	// Wait for another goroutine to grab it.
	goUnlock()
	<-locked
	takeDump()
	for _, tt := range []test{
		{n: 0, want: "M1> is locked by"},
		{n: 1},
	} {
		testFunc("3Gs removed", tt)
	}
	if failed {
		t.Logf("%s", buf.String())
	}

	// Tell the third goroutine to unlock.
	// All locks are now idle.
	goUnlock()
	takeDump()
	for _, tt := range []test{
		{n: 0},
	} {
		testFunc("all unlocked", tt)
	}
	if failed {
		t.Logf("%s", buf.String())
	}
}
