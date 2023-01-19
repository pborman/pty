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
	"testing"
	"time"
)

func TestSimple(t *testing.T) {
	m := New("this is a mutext")
	unlock := m.Lock("test lock")
	m.imu.Lock()
	if want, got := 0, len(m.waiters); want != got {
		t.Errorf("Got %d waiters, want %d", want, got)
	}
	m.imu.Unlock()
	c := make(chan struct{})
	go func() {
		unlock := m.Lock("Blocked")
		if want, got := 0, len(m.waiters); want != got {
			t.Errorf("Got %d waiters, want %d", want, got)
		}
		unlock()
		c <- struct{}{}
	}()
	time.Sleep(time.Second / 100)
	m.imu.Lock()
	if want, got := 1, len(m.waiters); want != got {
		t.Errorf("Got %d waiters, want %d", want, got)
	}
	m.imu.Unlock()
	unlock()
	<-c
}
