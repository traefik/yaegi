package main

import (
	"fmt"
	"iter"
	"maps"
	"slices"
)

type Number interface{ ~int | ~float64 }

func Sum[T Number](xs ...T) T {
	var s T
	for _, x := range xs {
		s += x
	}
	return s
}

type List[T any] struct{ items []T }

func (l *List[T]) Push(v T) { l.items = append(l.items, v) }

func (l *List[T]) All() iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for i, v := range l.items {
			if !yield(i, v) {
				return
			}
		}
	}
}

func Count(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range n {
			if !yield(i) {
				return
			}
		}
	}
}

func main() {
	fmt.Println(Sum(1, 2, 3), Sum(1.5, 2.5))
	l := &List[string]{}
	l.Push("a")
	l.Push("b")
	l.Push("c")
	for i, v := range l.All() {
		if i == 2 {
			break
		}
		fmt.Println(i, v)
	}
	var fns []func() int
	for i := range 3 { // per-iteration loop variable (Go 1.22)
		fns = append(fns, func() int { return i })
	}
	fmt.Println(fns[0](), fns[1](), fns[2]())
	total := 0
	for x := range Count(10) {
		if x%2 == 0 {
			continue
		}
		total += x
	}
	fmt.Println(total)
	s := []int{3, 1, 2}
	slices.Sort(s)
	fmt.Println(s, slices.Index(s, 2))
	m := map[string]int{"x": 1, "y": 2}
	keys := slices.Sorted(maps.Keys(m))
	fmt.Println(keys)
}
