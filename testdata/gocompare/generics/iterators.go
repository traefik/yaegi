// Iterators with generics: iter.Seq and iter.Seq2 producers, range over
// func with break, continue and early return, nested iterators, adapters
// (Map, Filter, Take, Zip), iter.Pull, and slices/maps iterator helpers.
package main

import (
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"
)

func Count(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 0; i < n; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

func Naturals() iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 1; ; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

func Map[T, U any](s iter.Seq[T], f func(T) U) iter.Seq[U] {
	return func(yield func(U) bool) {
		for v := range s {
			if !yield(f(v)) {
				return
			}
		}
	}
}

func Filter[T any](s iter.Seq[T], keep func(T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range s {
			if keep(v) && !yield(v) {
				return
			}
		}
	}
}

func Take[T any](s iter.Seq[T], n int) iter.Seq[T] {
	return func(yield func(T) bool) {
		if n <= 0 {
			return
		}
		i := 0
		for v := range s {
			if !yield(v) {
				return
			}
			i++
			if i == n {
				return
			}
		}
	}
}

func Enumerate[T any](s iter.Seq[T]) iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		i := 0
		for v := range s {
			if !yield(i, v) {
				return
			}
			i++
		}
	}
}

func Zip[A, B any](a iter.Seq[A], b iter.Seq[B]) iter.Seq2[A, B] {
	return func(yield func(A, B) bool) {
		nb, stop := iter.Pull(b)
		defer stop()
		for va := range a {
			vb, ok := nb()
			if !ok || !yield(va, vb) {
				return
			}
		}
	}
}

type Tree[T any] struct {
	Left, Right *Tree[T]
	Val         T
}

func (t *Tree[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) { t.push(yield) }
}

func (t *Tree[T]) push(yield func(T) bool) bool {
	if t == nil {
		return true
	}
	return t.Left.push(yield) && yield(t.Val) && t.Right.push(yield)
}

func find(s iter.Seq[int], target int) (int, bool) {
	steps := 0
	for v := range s {
		steps++
		if v == target {
			return steps, true // early return from range over func
		}
	}
	return steps, false
}

func main() {
	for i := range Count(3) {
		fmt.Print(i, " ")
	}
	fmt.Println()
	fmt.Println(slices.Collect(Take(Filter(Naturals(), func(n int) bool { return n%3 == 0 }), 5)))
	fmt.Println(slices.Collect(Map(Count(4), func(i int) string { return strings.Repeat("*", i) })))
	for i, v := range Enumerate(Map(Count(3), func(i int) float64 { return float64(i) / 4 })) {
		fmt.Print(i, ":", v, " ")
	}
	fmt.Println()
	for a, b := range Zip(Count(10), slices.Values([]string{"x", "y", "z"})) {
		fmt.Print(a, b, " ")
	}
	fmt.Println()

	// break, continue, labels, defer inside the loop body.
	sum := 0
outer:
	for i := range Naturals() {
		if i > 6 {
			break
		}
		for j := range Count(i) {
			if j == 2 {
				continue outer
			}
			sum += j
		}
	}
	fmt.Println(sum)
	fmt.Println(find(Naturals(), 7))
	fmt.Println(find(Count(5), 7))

	t := &Tree[string]{Val: "m", Left: &Tree[string]{Val: "c", Right: &Tree[string]{Val: "f"}}, Right: &Tree[string]{Val: "x"}}
	fmt.Println(slices.Collect(t.All()))
	for v := range t.All() {
		if v == "f" {
			break
		}
		fmt.Print(v, " ")
	}
	fmt.Println()

	next, stop := iter.Pull(Count(3))
	for {
		v, ok := next()
		if !ok {
			break
		}
		fmt.Print(v, " ")
	}
	stop()
	v, ok := next()
	fmt.Println(v, ok)

	next2, stop2 := iter.Pull2(maps.All(map[string]int{"only": 1}))
	k, val, ok := next2()
	fmt.Println(k, val, ok)
	stop2()

	m := map[string]int{"a": 1, "b": 2, "c": 3}
	fmt.Println(slices.Sorted(maps.Keys(m)))
	m2 := maps.Collect(Zip(slices.Values([]string{"p", "q"}), Count(2)))
	fmt.Println(m2)
	for i, s := range slices.Backward([]string{"a", "b", "c"}) {
		fmt.Print(i, s, " ")
	}
	fmt.Println()
	for c := range slices.Chunk([]int{1, 2, 3, 4, 5}, 2) {
		fmt.Print(c, " ")
	}
	fmt.Println()
	for i, w := range slices.All(strings.Fields("go vm test")) {
		fmt.Print(i, w, " ")
	}
	fmt.Println()
	for part := range strings.SplitSeq("a,b,,c", ",") {
		fmt.Printf("%q ", part)
	}
	fmt.Println()

	// Range over int and over a func with no values.
	var n int
	for range 3 {
		n++
	}
	var noVals func(func() bool) = func(yield func() bool) {
		for range 2 {
			if !yield() {
				return
			}
		}
	}
	for range noVals {
		n += 10
	}
	fmt.Println(n)

	// A panic in the loop body goes through the iterator.
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		for i := range Count(5) {
			if i == 2 {
				panic(fmt.Sprint("body panic at ", i))
			}
		}
	}()
}
