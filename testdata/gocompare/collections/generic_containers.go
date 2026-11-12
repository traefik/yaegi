// Generic collection code: a Set, a Stack and a Queue with type
// parameters, Map/Filter/Reduce over slices, a generic ordered map with
// sorted keys, and generic functions on arrays and maps.
package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

type Set[T comparable] map[T]struct{}

func (s Set[T]) Add(vs ...T) {
	for _, v := range vs {
		s[v] = struct{}{}
	}
}
func (s Set[T]) Has(v T) bool { _, ok := s[v]; return ok }

func SortedOf[T cmp.Ordered](s Set[T]) []T {
	out := make([]T, 0, len(s))
	for v := range s {
		out = append(out, v)
	}
	slices.Sort(out)
	return out
}

func Union[T comparable](a, b Set[T]) Set[T] {
	out := Set[T]{}
	for v := range a {
		out.Add(v)
	}
	for v := range b {
		out.Add(v)
	}
	return out
}

func Inter[T comparable](a, b Set[T]) Set[T] {
	out := Set[T]{}
	for v := range a {
		if b.Has(v) {
			out.Add(v)
		}
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

type Queue[T any] struct{ head, tail []T }

func (q *Queue[T]) Put(v T) { q.tail = append(q.tail, v) }
func (q *Queue[T]) Get() (T, bool) {
	if len(q.head) == 0 {
		for len(q.tail) > 0 {
			n := len(q.tail) - 1
			q.head = append(q.head, q.tail[n])
			q.tail = q.tail[:n]
		}
	}
	var zero T
	if len(q.head) == 0 {
		return zero, false
	}
	n := len(q.head) - 1
	v := q.head[n]
	q.head = q.head[:n]
	return v, true
}

func Map[T, U any](s []T, f func(T) U) []U {
	out := make([]U, len(s))
	for i, v := range s {
		out[i] = f(v)
	}
	return out
}

func Filter[S ~[]E, E any](s S, keep func(E) bool) S {
	var out S
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

func SumArr[T int | float64](a [4]T) T {
	var s T
	for _, v := range a {
		s += v
	}
	return s
}

func Invert[K, V comparable](m map[K]V) map[V]K {
	out := make(map[V]K, len(m))
	for k, v := range m {
		out[v] = k
	}
	return out
}

func GroupBy[T any, K comparable](s []T, key func(T) K) map[K][]T {
	out := map[K][]T{}
	for _, v := range s {
		k := key(v)
		out[k] = append(out[k], v)
	}
	return out
}

type Names []string

func main() {
	a := Set[int]{}
	a.Add(1, 2, 3, 4)
	b := Set[int]{}
	b.Add(3, 4, 5)
	fmt.Println(SortedOf(Union(a, b)), SortedOf(Inter(a, b)), a.Has(5), len(a))
	ss := Set[string]{}
	ss.Add(strings.Fields("the cat and the hat and the bat")...)
	fmt.Println(SortedOf(ss), len(ss))

	var st Stack[string]
	for _, w := range []string{"a", "b", "c"} {
		st.Push(w)
	}
	for {
		v, ok := st.Pop()
		if !ok {
			break
		}
		fmt.Print(v)
	}
	v, ok := st.Pop()
	fmt.Printf(" %q %v\n", v, ok)

	var q Queue[[2]int]
	q.Put([2]int{1, 1})
	q.Put([2]int{2, 2})
	x, _ := q.Get()
	q.Put([2]int{3, 3})
	y, _ := q.Get()
	z, _ := q.Get()
	_, ok = q.Get()
	fmt.Println(x, y, z, ok)

	nums := []int{1, 2, 3, 4, 5, 6}
	sq := Map(nums, func(n int) int { return n * n })
	strs := Map(sq, func(n int) string { return fmt.Sprint(n) })
	fmt.Println(sq, strings.Join(strs, ","))
	even := Filter(nums, func(n int) bool { return n%2 == 0 })
	fmt.Println(even, Filter(nums, func(int) bool { return false }) == nil)
	names := Names{"ann", "bob", "al"}
	an := Filter(names, func(s string) bool { return strings.HasPrefix(s, "a") })
	fmt.Printf("%v %T\n", an, an)
	fmt.Println(Reduce(nums, 0, func(a, n int) int { return a + n }), Reduce(nums, "", func(a string, n int) string { return a + fmt.Sprint(n) }))

	fmt.Println(SumArr([4]int{1, 2, 3, 4}), SumArr([4]float64{0.5, 0.25, 0.125, 0.125}))
	inv := Invert(map[string]int{"a": 1, "b": 2})
	fmt.Println(inv)
	g := GroupBy([]string{"apple", "bob", "avocado", "cat", "banana"}, func(s string) byte { return s[0] })
	fmt.Println(g)
	byLen := GroupBy([]string{"aa", "b", "cc", "ddd"}, func(s string) int { return len(s) })
	fmt.Println(byLen, len(byLen))
}
