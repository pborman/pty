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
