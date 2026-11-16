//go:build go1.27

// Generic methods (Go 1.27) whose result is their own generic type with a
// new type argument: the load-time check walked the signature of Map and
// got Box[R] with a new R each time, so the load never ended (yaegi -e and
// vm.Load; yaegi on a file has no check of exported methods).
package main

import (
	"fmt"
	"strconv"
)

type Box[T any] struct{ v T }

func (b Box[T]) Map[R any](f func(T) R) Box[R] { return Box[R]{f(b.v)} }

func (b Box[T]) Get() T { return b.v }

type E struct{ Box[int] }

type List[T any] struct{ items []T }

func (l *List[T]) Push(v T) { l.items = append(l.items, v) }

func (l List[T]) Fold[A any](a A, f func(A, T) A) A {
	for _, v := range l.items {
		a = f(a, v)
	}
	return a
}

func (l List[T]) MapL[R any](f func(T) R) List[R] {
	var out List[R]
	for _, v := range l.items {
		out.Push(f(v))
	}
	return out
}

type Plain struct{ n int }

func (p Plain) Twice[T any](t T) [2]T { return [2]T{t, t} }

func Run() any { return Box[int]{} }

func main() {
	var xs []any
	xs = append(xs, Run(), E{Box[int]{4}}, &E{}, Box[string]{"s"}, List[int]{}, Plain{})
	for _, x := range xs {
		fmt.Printf("%T %v\n", x, x)
	}
	b := Box[int]{21}.Map(func(i int) string { return strconv.Itoa(2 * i) })
	fmt.Println(b.Get(), b.Map(func(s string) int { return len(s) }).Get())
	e := E{Box[int]{7}}
	fmt.Println(e.Map(func(i int) bool { return i > 5 }).Get())
	var l List[int]
	for i := 1; i <= 4; i++ {
		l.Push(i)
	}
	fmt.Println(l.Fold(0, func(a, v int) int { return a + v }))
	fmt.Println(l.MapL(func(v int) string { return fmt.Sprint(v * v) }).Fold("", func(a, v string) string { return a + v + "," }))
	fmt.Println(Plain{}.Twice("x"), Plain{}.Twice[int](3))
	f := Box[int]{5}.Map[string]
	fmt.Println(f(strconv.Itoa).Get())
}
