// The run-time panics that a program recovers have the messages and the
// types of gc: close of a nil channel, assignment to a nil map (also through
// reflect), a negative shift, and the *reflect.ValueError of reflect, which
// a type assertion finds.
package main

import (
	"fmt"
	"reflect"
	"runtime"
)

func try(name string, f func()) {
	defer func() {
		r := recover()
		_, isVE := r.(*reflect.ValueError)
		_, isRE := r.(runtime.Error)
		fmt.Printf("%s: %T %v valueError=%v runtimeError=%v\n", name, r, r, isVE, isRE)
		if ve, ok := r.(*reflect.ValueError); ok {
			fmt.Println("  method", ve.Method, "kind", ve.Kind)
		}
	}()
	f()
}

var shift = -1

func main() {
	try("close nil", func() {
		var c chan int
		close(c)
	})
	try("close closed", func() {
		c := make(chan int)
		close(c)
		close(c)
	})
	try("send closed", func() {
		c := make(chan int, 1)
		close(c)
		c <- 1
	})
	try("nil map", func() {
		var m map[string]int
		m["a"] = 1
	})
	try("reflect nil map", func() {
		var m map[string]int
		reflect.ValueOf(m).SetMapIndex(reflect.ValueOf("a"), reflect.ValueOf(1))
	})
	try("reflect nil map delete", func() {
		var m map[string]int
		reflect.ValueOf(m).SetMapIndex(reflect.ValueOf("a"), reflect.Value{})
	})
	try("negative shift", func() {
		_ = 1 << shift
	})
	try("elem", func() { reflect.ValueOf(1).Elem() })
	try("index", func() { reflect.ValueOf(1).Index(0) })
}
