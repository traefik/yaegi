// Generic types: a stack, a queue, an ordered set, a pair, a generic linked
// list and a cache with methods; instantiation with several type arguments,
// generic types in maps and slices, and values of different instances.
package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

type Stack[T any] struct{ items []T }

func (s *Stack[T]) Push(v ...T) { s.items = append(s.items, v...) }
func (s *Stack[T]) Pop() (T, bool) {
	var zero T
	if len(s.items) == 0 {
		return zero, false
	}
	v := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return v, true
}
func (s Stack[T]) Len() int { return len(s.items) }

type Queue[T any] struct {
	head, tail *node[T]
	n          int
}

type node[T any] struct {
	v    T
	next *node[T]
}

func (q *Queue[T]) Put(v T) {
	nd := &node[T]{v: v}
	if q.tail == nil {
		q.head, q.tail = nd, nd
	} else {
		q.tail.next = nd
		q.tail = nd
	}
	q.n++
}

func (q *Queue[T]) Get() (T, bool) {
	if q.head == nil {
		var z T
		return z, false
	}
	v := q.head.v
	q.head = q.head.next
	if q.head == nil {
		q.tail = nil
	}
	q.n--
	return v, true
}

type Set[T cmp.Ordered] struct{ m map[T]struct{} }

func NewSet[T cmp.Ordered](xs ...T) Set[T] {
	s := Set[T]{m: map[T]struct{}{}}
	for _, x := range xs {
		s.Add(x)
	}
	return s
}
func (s Set[T]) Add(x T)      { s.m[x] = struct{}{} }
func (s Set[T]) Has(x T) bool { _, ok := s.m[x]; return ok }
func (s Set[T]) Sorted() []T {
	out := make([]T, 0, len(s.m))
	for k := range s.m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
func (s Set[T]) Union(o Set[T]) Set[T] {
	r := NewSet[T]()
	for k := range s.m {
		r.Add(k)
	}
	for k := range o.m {
		r.Add(k)
	}
	return r
}

type Pair[K comparable, V any] struct {
	Key K
	Val V
}

func (p Pair[K, V]) String() string { return fmt.Sprintf("%v=>%v", p.Key, p.Val) }
func (p Pair[K, V]) Swap() Pair[string, K] {
	return Pair[string, K]{fmt.Sprint(p.Val), p.Key}
}

type Cache[K comparable, V any] struct {
	data  map[K]V
	order []K
	cap   int
}

func NewCache[K comparable, V any](n int) *Cache[K, V] {
	return &Cache[K, V]{data: make(map[K]V), cap: n}
}

func (c *Cache[K, V]) Put(k K, v V) {
	if _, ok := c.data[k]; !ok {
		c.order = append(c.order, k)
		if len(c.order) > c.cap {
			delete(c.data, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.data[k] = v
}

func (c *Cache[K, V]) Get(k K) (V, bool) { v, ok := c.data[k]; return v, ok }

type Matrix[T ~int | ~float64] [][]T

func (m Matrix[T]) Mul(o Matrix[T]) Matrix[T] {
	r := make(Matrix[T], len(m))
	for i := range m {
		r[i] = make([]T, len(o[0]))
		for j := range o[0] {
			for k := range o {
				r[i][j] += T(m[i][k] * o[k][j]) // no FMA: round the product
			}
		}
	}
	return r
}

type Opt[T any] struct {
	v  T
	ok bool
}

func Some[T any](v T) Opt[T] { return Opt[T]{v, true} }
func None[T any]() Opt[T]    { return Opt[T]{} }
func (o Opt[T]) OrElse(d T) T {
	if o.ok {
		return o.v
	}
	return d
}

func main() {
	var s Stack[string]
	s.Push("a", "b", "c")
	v, ok := s.Pop()
	fmt.Println(v, ok, s.Len())
	si := &Stack[[]int]{}
	si.Push([]int{1}, nil)
	x, _ := si.Pop()
	fmt.Println(x == nil, si.Len())
	var empty Stack[float64]
	z, ok := empty.Pop()
	fmt.Println(z, ok)

	var q Queue[Pair[string, int]]
	q.Put(Pair[string, int]{"a", 1})
	q.Put(Pair[string, int]{"b", 2})
	p, _ := q.Get()
	fmt.Println(p, q.n)
	p, _ = q.Get()
	_, ok = q.Get()
	fmt.Println(p, ok, q.n)

	s1 := NewSet(3, 1, 2)
	s2 := NewSet(2, 5)
	fmt.Println(s1.Sorted(), s1.Union(s2).Sorted(), s1.Has(5), s2.Has(5))
	ss := NewSet("b", "a")
	fmt.Println(ss.Sorted())

	pr := Pair[int, []string]{7, []string{"x"}}
	fmt.Println(pr, pr.Swap(), pr.Swap().Swap())
	fmt.Printf("%v %s\n", Pair[string, bool]{"t", true}, Pair[rune, byte]{'a', 'b'})

	c := NewCache[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("a", 10)
	c.Put("c", 3)
	_, okA := c.Get("a")
	vb, okB := c.Get("b")
	vc, _ := c.Get("c")
	fmt.Println(okA, vb, okB, vc, c.order)

	m := Matrix[int]{{1, 2}, {3, 4}}
	fmt.Println(m.Mul(m), Matrix[float64]{{0.5}}.Mul(Matrix[float64]{{4}}))

	fmt.Println(Some(3).OrElse(0), None[int]().OrElse(-1), None[string]().OrElse("d"), Some("x").OrElse("y"))

	// Generic types in maps and slices.
	stacks := map[string]*Stack[int]{}
	for _, w := range strings.Fields("a b a c a") {
		if stacks[w] == nil {
			stacks[w] = &Stack[int]{}
		}
		stacks[w].Push(len(stacks[w].items))
	}
	for _, k := range []string{"a", "b", "c"} {
		fmt.Print(k, stacks[k].items, " ")
	}
	fmt.Println()
	pairs := []Pair[string, int]{{"z", 1}, {"a", 3}, {"m", 2}}
	slices.SortFunc(pairs, func(a, b Pair[string, int]) int { return cmp.Compare(a.Key, b.Key) })
	fmt.Println(pairs)
	opts := []Opt[int]{Some(1), None[int](), Some(3)}
	sum := 0
	for _, o := range opts {
		sum += o.OrElse(100)
	}
	fmt.Println(sum)

	// Comparison of generic struct values.
	fmt.Println(Pair[int, string]{1, "a"} == Pair[int, string]{1, "a"}, Opt[int]{} == None[int]())
}
