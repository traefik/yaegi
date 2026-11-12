// panic(nil) panics with a *runtime.PanicNilError since Go 1.21: recover()
// does not return nil.
package main

import (
	"errors"
	"fmt"
	"runtime"
)

func direct() (s string) {
	defer func() {
		r := recover()
		s = fmt.Sprintf("%T %v %v", r, r, r != nil)
	}()
	panic(nil)
}

// A nil interface of another static type is converted to any: still nil.
func typedNilIface() (s string) {
	defer func() {
		r := recover()
		s = fmt.Sprintf("%T %v", r, r)
	}()
	var err error
	panic(err)
}

// A nil pointer in an interface is not a nil interface: no PanicNilError.
func nilPointer() (s string) {
	defer func() {
		r := recover()
		p, ok := r.(*int)
		s = fmt.Sprint(r != nil, ok, p == nil)
	}()
	var p *int
	panic(p)
}

// defer panic(nil): the panic of a deferred builtin call.
func deferred() (s string) {
	defer func() {
		r := recover()
		s = fmt.Sprintf("%T %v", r, r)
	}()
	defer panic(nil)
	return "not reached"
}

func asError() string {
	var out string
	func() {
		defer func() {
			r := recover()
			err, ok := r.(error)
			var pe *runtime.PanicNilError
			_, isRT := r.(runtime.Error)
			out = fmt.Sprint(ok, errors.As(err, &pe), pe != nil, isRT, err)
		}()
		panic(nil)
	}()
	return out
}

// A panic(nil) replaces an earlier panic; a re-panic of the recovered
// value keeps the *runtime.PanicNilError.
func repanic() (s string) {
	defer func() {
		r := recover()
		s = fmt.Sprintf("%T %v", r, r)
	}()
	defer func() {
		r := recover()
		panic(r)
	}()
	defer func() { panic(nil) }()
	panic("first")
}

func inGoroutine() string {
	ch := make(chan string)
	go func() {
		defer func() {
			r := recover()
			ch <- fmt.Sprintf("%T %v", r, r)
		}()
		panic(nil)
	}()
	return <-ch
}

// The usual check: r != nil sees the panic.
func check() string {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("recovered:", r)
		}
	}()
	panic(nil)
}

func main() {
	fmt.Println(direct())
	fmt.Println(typedNilIface())
	fmt.Println(nilPointer())
	fmt.Println(deferred())
	fmt.Println(asError())
	fmt.Println(repanic())
	fmt.Println(inGoroutine())
	check()
}
