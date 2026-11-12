// Generic recursion: recursive generic functions (merge sort, flatten,
// power set), recursive generic types (tree with Insert and Fold, list),
// mutually recursive generic functions, and a generic memoized function.
package main

import (
	"cmp"
	"fmt"
	"strings"
)

func MergeSort[T cmp.Ordered](xs []T) []T {
	if len(xs) <= 1 {
		return xs
	}
	m := len(xs) / 2
	a, b := MergeSort(xs[:m]), MergeSort(xs[m:])
	out := make([]T, 0, len(xs))
	for len(a) > 0 && len(b) > 0 {
		if b[0] < a[0] {
			out, b = append(out, b[0]), b[1:]
		} else {
			out, a = append(out, a[0]), a[1:]
		}
	}
	return append(append(out, a...), b...)
}

func PowerSet[T any](xs []T) [][]T {
	if len(xs) == 0 {
		return [][]T{{}}
	}
	rest := PowerSet(xs[1:])
	out := append([][]T{}, rest...)
	for _, r := range rest {
		out = append(out, append([]T{xs[0]}, r...))
	}
	return out
}

type Tree[T cmp.Ordered] struct {
	Left, Right *Tree[T]
	Val         T
}

func (t *Tree[T]) Insert(v T) *Tree[T] {
	if t == nil {
		return &Tree[T]{Val: v}
	}
	switch cmp.Compare(v, t.Val) {
	case -1:
		t.Left = t.Left.Insert(v)
	case 1:
		t.Right = t.Right.Insert(v)
	}
	return t
}

func Fold[T cmp.Ordered, A any](t *Tree[T], acc A, f func(A, T) A) A {
	if t == nil {
		return acc
	}
	acc = Fold(t.Left, acc, f)
	acc = f(acc, t.Val)
	return Fold(t.Right, acc, f)
}

type List[T any] struct {
	Head T
	Tail *List[T]
}

func Cons[T any](h T, t *List[T]) *List[T] { return &List[T]{h, t} }

func (l *List[T]) String() string {
	if l == nil {
		return "nil"
	}
	return fmt.Sprint(l.Head) + ":" + l.Tail.String()
}

func MapList[T, U any](l *List[T], f func(T) U) *List[U] {
	if l == nil {
		return nil
	}
	return Cons(f(l.Head), MapList(l.Tail, f))
}

func IsEven[T ~int | ~uint](n T) bool {
	if n == 0 {
		return true
	}
	return IsOdd(n - 1)
}

func IsOdd[T ~int | ~uint](n T) bool {
	if n == 0 {
		return false
	}
	return IsEven(n - 1)
}

func Memo[K comparable, V any](f func(func(K) V, K) V) func(K) V {
	cache := map[K]V{}
	var g func(K) V
	g = func(k K) V {
		if v, ok := cache[k]; ok {
			return v
		}
		v := f(g, k)
		cache[k] = v
		return v
	}
	return g
}

type Nested[T any] struct {
	V    T
	Kids []Nested[T]
}

func Depth[T any](n Nested[T]) int {
	d := 0
	for _, k := range n.Kids {
		d = max(d, Depth(k))
	}
	return d + 1
}

func main() {
	fmt.Println(MergeSort([]int{5, 2, 9, 1, 5, 6, -3}))
	fmt.Println(MergeSort(strings.Fields("the quick brown fox jumps")))
	fmt.Println(MergeSort([]float64{2.5, -1, 0}))
	fmt.Println(PowerSet([]string{"a", "b", "c"}))
	fmt.Println(len(PowerSet(make([]int, 6))))

	var t *Tree[string]
	for _, w := range strings.Fields("m d x a f z d") {
		t = t.Insert(w)
	}
	fmt.Println(Fold(t, "", func(a string, v string) string { return a + v }))
	fmt.Println(Fold(t, 0, func(a int, _ string) int { return a + 1 }))
	var ti *Tree[int]
	for i := range 10 {
		ti = ti.Insert((i * 7) % 10)
	}
	fmt.Println(Fold(ti, []int{}, func(a []int, v int) []int { return append(a, v) }))

	l := Cons(1, Cons(2, Cons(3, (*List[int])(nil))))
	fmt.Println(l, MapList(l, func(i int) string { return strings.Repeat("x", i) }))
	fmt.Println(IsEven(10), IsOdd(7), IsEven(uint(3)))

	fib := Memo(func(self func(int) int64, n int) int64 {
		if n < 2 {
			return int64(n)
		}
		return self(n-1) + self(n-2)
	})
	fmt.Println(fib(50), fib(90))

	n := Nested[int]{1, []Nested[int]{{2, nil}, {3, []Nested[int]{{4, []Nested[int]{{5, nil}}}}}}}
	fmt.Println(Depth(n), Depth(Nested[string]{}))
}
