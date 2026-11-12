package main

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"
)

// Objects that the program cannot reach any more must be collected by
// runtime.GC, even when a register of a live frame, a register of a
// returned frame or a uintptr still refers to them (GOROOT tests
// fixedbugs/issue24491b.go and typeparam/chans.go). An object converted to
// uintptr in the arguments of a //go:uintptrescapes call stays alive during
// that call.

type T struct{ name string }

var done = make(chan string, 10)

func newT(name string) *T {
	t := &T{name: name}
	runtime.SetFinalizer(t, func(t *T) { done <- t.name })
	return t
}

// wait runs the GC until a finalizer reports, or gives up.
func wait() string {
	for i := 0; i < 50; i++ {
		runtime.GC()
		select {
		case s := <-done:
			return s
		case <-time.After(20 * time.Millisecond):
		}
	}
	return "not collected"
}

// deadLocal: t is dead after its last use, before runtime.GC.
func deadLocal() {
	t := newT("dead local")
	fmt.Println("using", t.name)
	fmt.Println(wait())
}

// closure: the only reference was in a closure run by a goroutine that
// ended.
func closure() {
	t := newT("closure")
	ended := make(chan bool)
	go func() {
		_ = t.name
		ended <- true
	}()
	<-ended
	fmt.Println(wait())
}

func returned() *T { return newT("returned frame") }

// With //go:uintptrescapes, the object of uintptr(p) in the arguments stays
// alive during the call.
//
//go:noinline
//go:uintptrescapes
func inCall(p uintptr) string {
	runtime.GC()
	select {
	case s := <-done:
		return "collected too early: " + s
	case <-time.After(50 * time.Millisecond):
	}
	return "alive during the call"
}

// Same through a variadic parameter.
//
//go:noinline
//go:uintptrescapes
func inCallV(ps ...uintptr) string { return inCall(ps[0]) }

func setup() unsafe.Pointer {
	t := newT("uintptr")
	return unsafe.Pointer(t)
}

func main() {
	deadLocal()
	closure()
	_ = returned()
	fmt.Println(wait())
	fmt.Println(inCall(uintptr(setup())))
	fmt.Println(wait())
	fmt.Println(inCallV(uintptr(setup())))
	fmt.Println(wait())
	// Without the directive, a uintptr does not keep its object alive.
	u := uintptr(setup())
	fmt.Println(wait(), u != 0)
}
