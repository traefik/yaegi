package main

// The correct uses of runtime.SetFinalizer and sync.Cond that the checks of
// Options.PanicOnFatal (vm/fatal.go) must not refuse: a finalizer on the
// first field of a struct or the first element of a slice (the start of
// the object, as in Go), a finalizer with an interface parameter, a reset
// with nil then a new finalizer, a finalizer set again by the finalizer
// itself, and Wait with the lock of the Cond held (a Mutex, an RLocker).

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

type pair struct{ a, b int }

func main() {
	x := &pair{a: 1}
	runtime.SetFinalizer(&x.a, func(*int) {})
	runtime.SetFinalizer(&x.a, nil)
	s := make([]*int, 4)
	runtime.SetFinalizer(&s[0], func(**int) {})
	runtime.SetFinalizer(&pair{}, func(any) {})
	y := &pair{}
	runtime.SetFinalizer(y, func(*pair) {})
	runtime.SetFinalizer(y, nil)
	runtime.SetFinalizer(y, func(*pair) {})
	runtime.KeepAlive(x)
	runtime.KeepAlive(y)

	// A finalizer that sets itself again once: it runs twice.
	runs := make(chan int, 2)
	n := 0
	var again func(*pair)
	again = func(p *pair) {
		n++
		runs <- n
		if n == 1 {
			runtime.SetFinalizer(p, again)
		}
	}
	runtime.SetFinalizer(&pair{}, again)
	for got := 0; got < 2; {
		runtime.GC()
		select {
		case <-runs:
			got++
		case <-time.After(10 * time.Millisecond):
		}
	}
	fmt.Println("finalizer runs:", n)

	var mu sync.Mutex
	c := sync.NewCond(&mu)
	ready := false
	go func() { mu.Lock(); ready = true; c.Broadcast(); mu.Unlock() }()
	mu.Lock()
	for !ready {
		c.Wait()
	}
	mu.Unlock()
	var rw sync.RWMutex
	rc := sync.NewCond(rw.RLocker())
	done := false
	go func() { rw.Lock(); done = true; rw.Unlock(); rc.Broadcast() }()
	rw.RLock()
	for !done {
		rc.Wait()
	}
	rw.RUnlock()
	fmt.Println("cond ok")
}
