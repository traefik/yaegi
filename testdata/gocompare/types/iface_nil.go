// nil interfaces versus interfaces holding a nil pointer, slice, map or
// func: comparison with nil, %v and %T, methods on a nil pointer through
// an interface, and the classic error-returning trap.
package main

import (
	"errors"
	"fmt"
)

type E struct{}

func (e *E) Error() string {
	if e == nil {
		return "nil *E"
	}
	return "E"
}

type S interface{ M() string }
type impl struct{ n int }

func (p *impl) M() string {
	if p == nil {
		return "nil impl"
	}
	return fmt.Sprint("impl ", p.n)
}

type fn func() string

func (f fn) M() string {
	if f == nil {
		return "nil fn"
	}
	return f()
}

func mayFail(fail bool) error {
	var p *E
	if fail {
		p = &E{}
	}
	return p // never a nil interface
}

func mayFailGood(fail bool) error {
	if fail {
		return &E{}
	}
	return nil
}

func main() {
	var e error
	fmt.Println(e == nil, e)
	fmt.Printf("%v %T\n", e, e)

	var pe *E
	e = pe
	fmt.Println(e == nil, pe == nil, e.Error())
	fmt.Printf("%v %T\n", e, e)
	fmt.Println(e == (*E)(nil), e != error(nil))

	fmt.Println(mayFail(false) == nil, mayFail(true) == nil)
	fmt.Println(mayFailGood(false) == nil, mayFailGood(true) == nil)

	var s S
	fmt.Println(s == nil)
	s = (*impl)(nil)
	fmt.Println(s == nil, s.M())
	s = &impl{3}
	fmt.Println(s.M())
	s = fn(nil)
	fmt.Println(s == nil, s.M())
	s = fn(func() string { return "fn called" })
	fmt.Println(s.M())

	var a any
	fmt.Println(a == nil)
	for _, v := range []any{nil, (*int)(nil), []int(nil), map[string]int(nil), (func())(nil), (chan int)(nil), error(nil), (*E)(nil)} {
		fmt.Printf("%T isnil=%v ", v, v == nil)
	}
	fmt.Println()
	fmt.Printf("%v %v %v %v\n", []int(nil), map[string]int(nil), (*int)(nil), any(nil))

	var ip *int
	a = ip
	p2, ok := a.(*int)
	fmt.Println(ok, p2 == nil)
	_, ok = a.(error)
	fmt.Println(ok)

	var target *E
	fmt.Println(errors.As(error(&E{}), &target), target != nil)

	// A nil interface method call panics.
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		var ns S
		fmt.Println(ns.M())
	}()
	// Comparison between two interfaces holding nil pointers of different types.
	var x, y any = (*int)(nil), (*string)(nil)
	fmt.Println(x == y, x == any((*int)(nil)))
}
