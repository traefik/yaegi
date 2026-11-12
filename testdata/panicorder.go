// defer, panic and recover: order of deferred calls, panics in deferred
// calls (the new panic replaces the old one), recover only in the deferred
// function itself, re-panic, runtime errors as values, panics with
// interpreted error types, and defer in loops and closures.
package main

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

type myErr struct{ code int }

func (e *myErr) Error() string { return fmt.Sprint("myErr ", e.code) }

func order() (out []string) {
	defer func() { out = append(out, "d1") }()
	for i := range 3 {
		defer func() { out = append(out, fmt.Sprint("loop", i)) }()
	}
	defer func(s string) { out = append(out, s) }("arg evaluated early")
	return []string{"body"}
}

func replaced() (msg string) {
	defer func() {
		r := recover()
		msg = fmt.Sprint("recovered: ", r)
	}()
	defer func() {
		panic("second")
	}()
	panic("first")
}

func notDirect() (msg string) {
	defer func() {
		r := recover()
		msg = fmt.Sprint("outer got: ", r)
	}()
	defer func() {
		helper := func() any { return recover() } // not called directly by the deferred call: returns nil
		msg = fmt.Sprint("helper got: ", helper())
	}()
	panic("p")
}

func repanic() (msg string) {
	defer func() {
		msg = fmt.Sprint("final: ", recover())
	}()
	func() {
		defer func() {
			r := recover()
			panic(fmt.Sprint("wrapped(", r, ")"))
		}()
		panic("inner")
	}()
	return "not reached"
}

func runtimeErr(f func()) (s string) {
	defer func() {
		r := recover()
		if e, ok := r.(runtime.Error); ok {
			s = "runtime.Error: " + e.Error()
			return
		}
		if e, ok := r.(error); ok {
			var me *myErr
			if errors.As(e, &me) {
				s = fmt.Sprint("myErr code ", me.code)
				return
			}
			s = "error: " + e.Error()
			return
		}
		s = fmt.Sprintf("other %T: %v", r, r)
	}()
	f()
	return "no panic"
}

type T struct{ n int }

func (t *T) get() int { return t.n }

func deferNilFunc() (s string) {
	defer func() { s = fmt.Sprint(recover() != nil) }()
	var f func()
	defer f() // panics when the deferred call runs
	return "x"
}

func deferMethodValue() (out []int) {
	t := &T{1}
	defer func() { out = append(out, t.n) }()
	defer fmt.Sprint(t.get()) // receiver and args evaluated now
	t.n = 5
	return []int{t.get()}
}

func recoverReturnsNilAfter() string {
	var r1, r2 any
	func() {
		defer func() { r1 = recover(); r2 = recover() }()
		panic("once")
	}()
	return fmt.Sprint(r1, " ", r2)
}

func deepDefers(n int) (sum int) {
	defer func() { sum += n }()
	if n == 0 {
		panic("bottom")
	}
	return deepDefers(n-1) + 0
}

func main() {
	fmt.Println(order())
	fmt.Println(replaced())
	fmt.Println(notDirect())
	fmt.Println(repanic())

	var arr []int
	var m map[string]int
	var p *T
	var i any = "str"
	zero := 0
	fmt.Println(runtimeErr(func() { _ = arr[5] }))
	fmt.Println(runtimeErr(func() { m["a"] = 1 }))
	fmt.Println(runtimeErr(func() { _ = p.n }))
	fmt.Println(runtimeErr(func() { _ = i.(int) }))
	fmt.Println(runtimeErr(func() { _ = 10 / zero }))
	fmt.Println(runtimeErr(func() { _ = arr[1:zero] }))
	fmt.Println(runtimeErr(func() { panic(&myErr{42}) }))
	fmt.Println(runtimeErr(func() { panic(fmt.Errorf("wrap: %w", &myErr{7})) }))
	fmt.Println(runtimeErr(func() { panic(errors.New("plain")) }))
	fmt.Println(runtimeErr(func() { panic(struct{ A, B int }{1, 2}) }))
	fmt.Println(runtimeErr(func() { panic(nil) }))
	fmt.Println(runtimeErr(func() {
		ch := make(chan int)
		close(ch)
		close(ch)
	}))
	fmt.Println(runtimeErr(func() {
		ch := make(chan int, 1)
		close(ch)
		ch <- 1
	}))
	fmt.Println(runtimeErr(func() {
		s := make([]int, 2)
		_ = [3]int(s)
	}))
	fmt.Println(runtimeErr(func() {
		n := -1
		_ = make([]int, n)
	}))

	fmt.Println(deferNilFunc())
	fmt.Println(deferMethodValue())
	fmt.Println(recoverReturnsNilAfter())

	fmt.Println(runtimeErr(func() { deepDefers(50) }))
	s := runtimeErr(func() {
		defer func() {
			if r := recover(); r != nil {
				panic(strings.ToUpper(fmt.Sprint(r)))
			}
		}()
		panic("lower")
	})
	fmt.Println(s)
}
