package main

import (
	"fmt"
	"sort"
)

type Number interface{ ~int | ~int64 | ~float64 }

func Sum[T Number](xs []T) T {
	var s T
	for _, x := range xs {
		s += x
	}
	return s
}

func Map[T, U any](xs []T, f func(T) U) []U {
	out := make([]U, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

type Stack[T any] struct{ items []T }

func (s *Stack[T]) Push(v T) { s.items = append(s.items, v) }
func (s *Stack[T]) Pop() (T, bool) {
	var zero T
	if len(s.items) == 0 {
		return zero, false
	}
	v := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return v, true
}

func keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func main() {
	fmt.Println(Sum([]int{1, 2, 3, 4}))
	fmt.Println(Sum([]float64{1.5, 2.5}))
	doubled := Map([]int{1, 2, 3}, func(x int) int { return x * 2 })
	fmt.Println(doubled)
	strs := Map([]int{1, 2, 3}, func(x int) string { return fmt.Sprintf("n%d", x) })
	fmt.Println(strs)

	var st Stack[string]
	st.Push("a")
	st.Push("b")
	v, ok := st.Pop()
	fmt.Println(v, ok)

	m := map[string]int{"x": 1, "y": 2, "z": 3}
	ks := keys(m)
	sort.Strings(ks)
	fmt.Println(ks)
}
