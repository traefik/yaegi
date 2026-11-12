package main

import (
	"cmp"
	"fmt"
	"strings"
)

type Number interface {
	~int | ~int64 | ~float64
}

func Sum[T Number](xs ...T) T {
	var s T
	for _, x := range xs {
		s += x
	}
	return s
}

func Map[T, U any](xs []T, f func(T) U) []U {
	out := make([]U, 0, len(xs))
	for _, x := range xs {
		out = append(out, f(x))
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

type Pair[K comparable, V any] struct {
	Key K
	Val V
}

func (p Pair[K, V]) String() string { return fmt.Sprintf("%v=%v", p.Key, p.Val) }

func MaxOf[T cmp.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

type MyInt int

type Cache[K comparable, V any] struct{ m map[K]V }

func NewCache[K comparable, V any]() *Cache[K, V] { return &Cache[K, V]{m: map[K]V{}} }
func (c *Cache[K, V]) Get(k K) (V, bool)         { v, ok := c.m[k]; return v, ok }
func (c *Cache[K, V]) Set(k K, v V)              { c.m[k] = v }

func main() {
	fmt.Println(Sum(1, 2, 3), Sum(1.5, 2.5), Sum[MyInt](4, 5))
	fmt.Println(Map([]string{"a", "b"}, strings.ToUpper), Map([]int{1, 2}, func(i int) string { return fmt.Sprint(i * 2) }))
	var s Stack[string]
	s.Push("x")
	s.Push("y")
	v, ok := s.Pop()
	fmt.Println(v, ok)
	p := Pair[string, int]{"a", 1}
	fmt.Println(p, p.String())
	fmt.Println(MaxOf("b", "a"), MaxOf(3, 7))
	c := NewCache[string, []int]()
	c.Set("k", []int{1})
	fmt.Println(c.Get("k"))
}
