package anymap

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func ExampleNew() {
	am := New()

	// Use unique types to differentiate events
	type Name string
	type Role string

	type S struct {
		Value string
	}

	var (
		wg   sync.WaitGroup
		name Name
		role Role
		str  string
		age  int
		s    *S
	)
	// Sending values to the Anymap does not block
	am.C <- &S{Value: "value"}

	// Start up requests in the background.  They will block until an event
	// of the proper type is sent.
	wg.Add(4)
	go func() {
		am.Get(&name)
		wg.Done()
	}()
	go func() {
		am.Get(&role)
		wg.Done()
	}()
	go func() {
		am.Get(&age)
		wg.Done()
	}()
	go func() {
		am.Get(&str)
		wg.Done()
	}()

	// This will not block as the value was already set
	am.Get(&s)

	// Send the events.  The calls to Get will unblock.
	am.C <- "string"
	am.C <- Name("bob")
	am.C <- Role("farmer")
	am.C <- 42

	// Wait for all the calls to Get to return.
	wg.Wait()

	fmt.Printf("Name: %s\n", name)
	fmt.Printf("Role: %s\n", role)
	fmt.Printf("Age : %d\n", age)
	fmt.Printf("S   : %v\n", s.Value)
	fmt.Printf("Str : %v\n", str)

	// Output:
	// Name: bob
	// Role: farmer
	// Age : 42
	// S   : value
	// Str : string
}

func TestGet(t *testing.T) {
	am := New()

	// Use unique types to differentiate events
	type Name string
	type Role string

	type S struct {
		Value string
	}

	var (
		wg   sync.WaitGroup
		name Name
		role Role
		str  string
		age  int
		s    *S
	)
	// Sending values to the Anymap does not block
	am.C <- &S{Value: "value"}

	// Start up requests in the background.  They will block until an event
	// of the proper type is sent.
	wg.Add(4)
	go func() {
		am.Get(&name)
		wg.Done()
	}()
	go func() {
		am.Get(&role)
		wg.Done()
	}()
	go func() {
		am.Get(&age)
		wg.Done()
	}()
	go func() {
		am.Get(&str)
		wg.Done()
	}()
	time.Sleep(time.Millisecond)

	// This will not block as the value was already set
	am.Get(&s)

	// Send the events.  The calls to Get will unblock.
	am.C <- "string"
	am.C <- Name("bob")
	am.C <- Role("farmer")
	am.C <- 42

	// Wait for all the calls to Get to return.
	wg.Wait()

	fmt.Printf("Name: %s\n", name)
	fmt.Printf("Role: %s\n", role)
	fmt.Printf("Age : %d\n", age)
	fmt.Printf("S   : %v\n", s.Value)
	fmt.Printf("Str : %v\n", str)

	// Output:
	// Name: bob
	// Role: farmer
	// Age : 42
	// S   : value
	// Str : string
}

func TestNext(t *testing.T) {
	var os, s string
	am := New()
	am.C <- "value"
	am.Get(&os) // Wait for the value to be set
	go func() {
		time.Sleep(time.Second / 100)
		am.C <- "next"
	}()
	am.Next(&s)
}

func TestPeek(t *testing.T) {
	var s, s1 string
	am := New()
	if am.Peek(&s) {
		t.Errorf("Peek returned true when no value was set")
	}
	am.C <- "value"
	am.Get(&s1)
	if !am.Peek(&s) {
		t.Errorf("Peek returned false when value was set")
	}
	if s != "value" {
		t.Errorf("Peek got %q, want %q", s, "value")
	}
}

func TestLeak1(t *testing.T) {
	done := make(chan struct{})
	am := New()
	waitFor := func(n int64) {
		for atomic.LoadInt64(&am.goroutines) != n {
			time.Sleep(time.Microsecond)
		}
	}
	go func() {
		defer close(done)
		uc1 := am.newUpdateChan()
		uc2 := am.newUpdateChan()
		uc3 := am.newUpdateChan()
		am.C <- true // This will cause a goroutine to start for each uc
		waitFor(3)
		// The goroutines will block trying to send a value to uc.ch
		uc1.shutdown()
		waitFor(2)
		uc2.shutdown()
		waitFor(1)
		uc3.shutdown()
		waitFor(0)
		am.C <- true // cause watch to remove dead updateChans
		for {
			am.mu.Lock()
			n := len(am.updateChans)
			am.mu.Unlock()
			if n == 0 {
				break
			}
			time.Sleep(time.Microsecond)
		}
	}()
	select {
	case <-time.After(time.Second * 5):
		t.Fatalf("timeout waiting for goroutines %d/%d", atomic.LoadInt64(&am.goroutines), len(am.updateChans))
	case <-done:
	}
}

func TestLeak2(t *testing.T) {
	done := make(chan struct{})
	am := New()
	waitFor := func(n int64) {
		for atomic.LoadInt64(&am.goroutines) != n {
			time.Sleep(time.Microsecond)
		}
	}
	go func() {
		defer close(done)
		am.newUpdateChan()
		am.newUpdateChan()
		am.newUpdateChan()
		am.C <- true // This will cause a goroutine to start for each uc
		waitFor(3)
		close(am.C) // This will cause watch to return
		waitFor(0)
		for {
			am.mu.Lock()
			n := len(am.updateChans)
			am.mu.Unlock()
			if n == 0 {
				break
			}
			time.Sleep(time.Microsecond)
		}
	}()
	select {
	case <-time.After(time.Second * 5):
		t.Fatalf("timeout waiting for goroutines %d/%d", atomic.LoadInt64(&am.goroutines), len(am.updateChans))
	case <-done:
	}
}

// TestUnreached tests the final return of updateChan.next.  Normally this
// line cannot possibly be reached as updateChan.ch is never closed.  We close
// it in this test just to make sure if the impossible happens it happens the
// way we expect.  (And go test does appear to have a mechanism to say a return
// can never be reached.)
func TestUnreached(t *testing.T) {
	am := New()
	uc := am.newUpdateChan()
	done := make(chan struct{})
	result := true
	go func() {
		var s string
		result = uc.next(reflect.ValueOf(&s))
		close(done)
	}()
	close(uc.ch)
	<-done
	if result {
		t.Errorf("uc.next returned true when uc.ch was closed")
	}
}

func TestSet(t *testing.T) {
	var v reflect.Value
	// v.Set(v) will panic
	set(v, v)
}
