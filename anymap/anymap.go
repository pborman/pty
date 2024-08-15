// Package anymap provides an event map based on event types.  Events are sent
// to the Anymap.C channel and retrieved by Anymap.Get, Anymap.Next, or
// Anymap.Peek.  The map is keyed on the vents type.  The Get, Next, and Peek
// methods are passed a pointer to the event type requested.  Get, Next, and
// Peek will panic if not passed a pointer.
//
// When the value is a pointer (e.g., pointer to a structure) then a poiner to a
// pointer is passed to Get, Next, or Peek:
//
//	var value *SomeStruct
//	am.Get(&value) // passing a **SomeStruct
package anymap

import (
	"reflect"
	"sync"
	"sync/atomic"
)

// An Anymap receives values on C
type Anymap struct {
	mu          sync.Mutex
	C           chan<- any
	values      map[reflect.Type]any
	updateChans map[*updateChan]struct{}
	goroutines  int64 // used in tests to look for leaked goroutines
}

type updateChan struct {
	am   *Anymap
	mu   sync.Mutex
	ch   chan any
	done chan struct{}
}

func (uc *updateChan) next(v reflect.Value) bool {
	defer func() { go uc.close() }()
	want := v.Type()
	for value := range uc.ch {
		if reflect.TypeOf(value) != want {
			continue
		}
		set(v, value)
		return true
	}
	return false
}

// shutdown causes any goroutines associated with uc to terminate.
func (uc *updateChan) shutdown() {
	uc.mu.Lock()
	if uc.done != nil {
		close(uc.done)
		uc.done = nil
	}
	uc.mu.Unlock()
	type unused bool
}

func (uc *updateChan) close() {
	uc.shutdown()
	uc.am.mu.Lock()
	delete(uc.am.updateChans, uc)
	uc.am.mu.Unlock()
}

// set is a "safe" version of reflect.Value.Set.
func set(v reflect.Value, value any) {
	if v.CanSet() {
		v.Set(reflect.ValueOf(value))
	}
}

// New returns an initialized Anymap.  New values are sent to C.
func New() *Anymap {
	ch := make(chan any)
	am := &Anymap{
		C:           ch,
		values:      map[reflect.Type]any{},
		updateChans: map[*updateChan]struct{}{},
	}
	go am.watch(ch)
	return am
}

func (am *Anymap) chans() []*updateChan {
	var chans []*updateChan
	for uc := range am.updateChans {
		chans = append(chans, uc)
	}
	return chans
}

func (am *Anymap) watch(ch chan any) {
	for v := range ch {
		am.mu.Lock()
		am.values[reflect.TypeOf(v)] = v
		for _, uc := range am.chans() {
			uc.mu.Lock()
			done := uc.done
			uc.mu.Unlock()
			go func() {
				atomic.AddInt64(&am.goroutines, 1)
				select {
				case uc.ch <- v:
				case <-done:
					uc.close()
				}
				atomic.AddInt64(&am.goroutines, -1)
			}()
		}
		am.mu.Unlock()
	}
	am.mu.Lock()
	chans := am.chans()
	am.mu.Unlock()
	for _, uc := range chans {
		uc.close()
	}
}

func (am *Anymap) newUpdateChan() *updateChan {
	uc := &updateChan{
		am:   am,
		ch:   make(chan any),
		done: make(chan struct{}),
	}
	am.updateChans[uc] = struct{}{}
	return uc
}

// Get returns the current value of the type *a.  If no value of type *a has
// been sent Get will block until one is sent.
// Get returns false if anymap.C is closed with no type *a being sent.
func (am *Anymap) Get(a any) bool {
	v := reflect.ValueOf(a).Elem()
	want := v.Type()

	am.mu.Lock()
	value, ok := am.values[want]
	var uc *updateChan
	if ok {
		set(v, value)
	} else {
		uc = am.newUpdateChan()
	}
	am.mu.Unlock()

	if uc != nil {
		return uc.next(v)
	}
	return true
}

// Next returns the next value of type *a that is sent.  Any current value of
// type *a is ignored.  Next returns false if anymap.C is closed with no type *a
// being sent.
//
// Next should only be used for repeating events.  Next may indefiniately block
// if the value of type *a arrives prior to Next being called.
func (am *Anymap) Next(a any) bool {
	am.mu.Lock()
	uc := am.newUpdateChan()
	am.mu.Unlock()
	return uc.next(reflect.ValueOf(a).Elem())
}

// Peek returns the current value of type *a.  Peek returns true if a value
// is set, false otherwise.
//
// Note that the value of type *a may be set or change concurrently with Peek
// being called.
func (am *Anymap) Peek(a any) bool {
	v := reflect.ValueOf(a)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	am.mu.Lock()
	value, ok := am.values[v.Type()]
	am.mu.Unlock()
	if ok {
		set(v, value)
	}
	return ok
}
