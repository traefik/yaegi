// Generic functions: explicit and inferred instantiation, several type
// parameters, any and comparable, generic function values, partial
// instantiation with inference of the rest, and generic closures.
package main

import (
	"fmt"
	"strconv"
	"strings"
)

func Map[T, U any](s []T, f func(T) U) []U {
	out := make([]U, 0, len(s))
	for _, v := range s {
		out = append(out, f(v))
	}
	return out
}

func Filter[T any](s []T, keep func(T) bool) []T {
	var out []T
	for _, v := range s {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

func Reduce[T, A any](s []T, init A, f func(A, T) A) A {
	acc := init
	for _, v := range s {
		acc = f(acc, v)
	}
	return acc
}

func Index[T comparable](s []T, x T) int {
	for i, v := range s {
		if v == x {
			return i
		}
	}
	return -1
}

func Keys[K comparable, V any](m map[K]V) int { return len(m) }

func Swap[T any](a, b *T) { *a, *b = *b, *a }

func Convert[To, From any](v From, f func(From) To) To { return f(v) }

func Pair[A, B any](a A, b B) struct {
	A A
	B B
} {
	return struct {
		A A
		B B
	}{a, b}
}

func Repeat[T any](v T, n int) []T {
	s := make([]T, n)
	for i := range s {
		s[i] = v
	}
	return s
}

func Uniq[T comparable](s []T) []T {
	seen := map[T]bool{}
	var out []T
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func Apply[T any](v T, fs ...func(T) T) T {
	for _, f := range fs {
		v = f(v)
	}
	return v
}

func Counter[T comparable]() func(T) int {
	m := map[T]int{}
	return func(v T) int {
		m[v]++
		return m[v]
	}
}

type point struct{ X, Y int }

func main() {
	ints := []int{1, 2, 3, 4, 5, 6}
	fmt.Println(Map(ints, func(i int) string { return strconv.Itoa(i * i) }))
	fmt.Println(Map[int, float64](ints, func(i int) float64 { return float64(i) / 2 }))
	fmt.Println(Filter(ints, func(i int) bool { return i%2 == 0 }))
	fmt.Println(Reduce(ints, 0, func(a, b int) int { return a + b }))
	fmt.Println(Reduce(ints, "", func(a string, b int) string { return a + strconv.Itoa(b) }))
	fmt.Println(Index(ints, 4), Index([]string{"a", "b"}, "b"), Index([]string{}, "x"))
	fmt.Println(Index([]point{{1, 2}, {3, 4}}, point{3, 4}), Index([]any{1, "a", nil}, nil))
	fmt.Println(Keys(map[string]int{"a": 1, "b": 2}), Keys(map[point][]int{}))

	a, b := "x", "y"
	Swap(&a, &b)
	p1, p2 := point{1, 1}, point{2, 2}
	Swap(&p1, &p2)
	fmt.Println(a, b, p1, p2)

	fmt.Println(Convert[string](42, strconv.Itoa), Convert[int]("7", func(s string) int { n, _ := strconv.Atoi(s); return n }))
	pr := Pair("k", 3.5)
	fmt.Printf("%v %+v %T\n", pr, pr, pr)
	fmt.Println(Repeat("ab", 3), Repeat(point{}, 2), len(Repeat[int](0, 0)))
	fmt.Println(Uniq([]string{"a", "b", "a", "c", "b"}), Uniq([]int{3, 3, 3}))
	fmt.Println(Apply("go", strings.ToUpper, func(s string) string { return s + "!" }), Apply(3))

	// Generic function values.
	mapper := Map[string, int]
	fmt.Println(mapper([]string{"a", "bb", "ccc"}, func(s string) int { return len(s) }))
	idx := Index[float64]
	fmt.Println(idx([]float64{1.5, 2.5}, 2.5))
	fmt.Printf("%T\n", idx)

	c := Counter[string]()
	c("a")
	c("b")
	fmt.Println(c("a"), c("c"))
	ci := Counter[point]()
	fmt.Println(ci(point{}), ci(point{}), ci(point{1, 0}))

	// Inference through a named slice type keeps the named type.
	type Names []string
	n := Filter(Names{"ann", "bob", "al"}, func(s string) bool { return strings.HasPrefix(s, "a") })
	fmt.Printf("%T %v\n", n, n)
}
