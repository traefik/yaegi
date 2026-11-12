package main

import (
	"fmt"
	"iter"
	"maps"
	"slices"
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

func Enum[T any](xs []T) iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for i, x := range xs {
			if !yield(i, x) {
				return
			}
		}
	}
}

func main() {
	for i := range Count(5) {
		if i == 3 {
			break
		}
		fmt.Print(i, " ")
	}
	fmt.Println()
	for i, s := range Enum([]string{"a", "b"}) {
		fmt.Print(i, s, " ")
	}
	fmt.Println()
	next, stop := iter.Pull(Count(3))
	defer stop()
	a, ok := next()
	fmt.Println(a, ok)
	m := map[string]int{"z": 1, "a": 2}
	fmt.Println(slices.Sorted(maps.Keys(m)), slices.Collect(Count(3)))
}
