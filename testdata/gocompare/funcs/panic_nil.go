// panic(nil) since Go 1.21: recover returns a *runtime.PanicNilError with its
// message; a nil error or nil pointer passed to panic; recover's result
// compared with nil; Unwrap/RuntimeError on the value.
package main

import (
	"errors"
	"fmt"
	"runtime"
)

type E struct{}

func (*E) Error() string { return "E" }

func show(name string, f func()) {
	defer func() {
		r := recover()
		fmt.Printf("%s: nil=%v type=%T value=%v\n", name, r == nil, r, r)
		if pe, ok := r.(*runtime.PanicNilError); ok {
			fmt.Println("  PanicNilError:", pe.Error())
			var re runtime.Error = pe
			fmt.Println("  is runtime.Error:", re != nil)
			var target *runtime.PanicNilError
			fmt.Println("  errors.As:", errors.As(pe, &target))
		}
	}()
	f()
}

func main() {
	show("panic(nil)", func() { panic(nil) })
	show("panic(interface nil)", func() {
		var i interface{}
		panic(i)
	})
	show("panic(nil error)", func() {
		var err error
		panic(err)
	})
	show("panic(nil *E)", func() {
		var e *E
		panic(e)
	})
	show("panic(nil *E as error)", func() {
		var e *E
		var err error = e
		panic(err)
	})
	show("panic(nil slice)", func() {
		var s []int
		panic(s)
	})
	show("panic(nil map)", func() {
		var m map[string]int
		panic(m)
	})
	show("no panic", func() {})
	// Deferred functions still run after panic(nil).
	func() {
		defer func() { fmt.Println("recovered after panic(nil):", recover() != nil) }()
		defer fmt.Println("defer runs before recover")
		panic(nil)
	}()
}
