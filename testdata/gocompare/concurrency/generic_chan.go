// Generic code over channels: generic types with channel fields, generic
// functions with channel type parameters and constraints on channel types,
// channels of generic structs, and a generic future/promise.
package main

import (
	"fmt"
	"sort"
	"strings"
)

type Pair[K comparable, V any] struct {
	Key K
	Val V
}

type Future[T any] struct {
	ch chan T
}

func Async[T any](f func() T) *Future[T] {
	fu := &Future[T]{ch: make(chan T, 1)}
	go func() { fu.ch <- f() }()
	return fu
}

func (f *Future[T]) Get() T {
	v := <-f.ch
	f.ch <- v
	return v
}

type Queue[T any] struct {
	c chan T
}

func NewQueue[T any](n int) *Queue[T] { return &Queue[T]{c: make(chan T, n)} }
func (q *Queue[T]) Push(v T) bool {
	select {
	case q.c <- v:
		return true
	default:
		return false
	}
}
func (q *Queue[T]) Pop() (T, bool) {
	select {
	case v := <-q.c:
		return v, true
	default:
		var zero T
		return zero, false
	}
}
func (q *Queue[T]) Len() int { return len(q.c) }

// Drain works on any receive channel type with the element E.
func Drain[C ~chan E | ~<-chan E, E any](c C) []E {
	var r []E
	for v := range c {
		r = append(r, v)
	}
	return r
}

type IntChan chan int

func FromSlice[T any](s []T) <-chan T {
	c := make(chan T, len(s))
	for _, v := range s {
		c <- v
	}
	close(c)
	return c
}

func Reduce[T, A any](c <-chan T, init A, f func(A, T) A) A {
	acc := init
	for v := range c {
		acc = f(acc, v)
	}
	return acc
}

func main() {
	f1 := Async(func() int { return 6 * 7 })
	f2 := Async(func() string { return strings.Repeat("ab", 3) })
	f3 := Async(func() Pair[string, []int] { return Pair[string, []int]{"k", []int{1, 2}} })
	fmt.Println(f1.Get(), f1.Get(), f2.Get(), f3.Get())

	q := NewQueue[string](2)
	fmt.Println(q.Push("a"), q.Push("b"), q.Push("c"), q.Len())
	v, ok := q.Pop()
	fmt.Println(v, ok)
	v, ok = q.Pop()
	fmt.Println(v, ok)
	v, ok = q.Pop()
	fmt.Printf("%q %v\n", v, ok)

	qf := NewQueue[float64](1)
	qf.Push(2.5)
	x, ok := qf.Pop()
	fmt.Println(x, ok)

	ic := make(IntChan, 3)
	ic <- 1
	ic <- 2
	ic <- 3
	close(ic)
	fmt.Println(Drain(ic))
	fmt.Println(Drain(FromSlice([]string{"p", "q"})))

	sum := Reduce(FromSlice([]int{1, 2, 3, 4}), 0, func(a, b int) int { return a + b })
	joined := Reduce(FromSlice([]string{"x", "y", "z"}), "", func(a, b string) string { return a + b })
	fmt.Println(sum, joined)

	// Channel of generic pairs, filled by goroutines, sorted.
	pc := make(chan Pair[string, int])
	words := []string{"delta", "alpha", "charlie", "bravo"}
	for _, w := range words {
		go func() { pc <- Pair[string, int]{w, len(w)} }()
	}
	var ps []Pair[string, int]
	for range words {
		ps = append(ps, <-pc)
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].Key < ps[j].Key })
	fmt.Printf("%v %+v\n", ps, ps[0])
	fmt.Printf("%T\n", pc)
}
