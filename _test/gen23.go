package main

import "fmt"

type Node[K comparable, V any] interface {
	Key() K
	Value() V
}

type node[K comparable, V any] struct {
	key K
	val V
}

func (n *node[K, V]) Key() K { return n.key }

func (n *node[K, V]) Value() V { return n.val }

func Equals[K comparable, V any](a, b Node[K, V]) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Key() == b.Key()
}

func Keys[K comparable, V any](m map[K]V) int { return len(m) }

func Apply[T, U any](f func(T) U, v ...T) []U {
	var r []U
	for _, x := range v {
		r = append(r, f(x))
	}
	return r
}

func main() {
	var a Node[string, int] = &node[string, int]{key: "a"}
	var b Node[string, int] = &node[string, int]{key: "a"}
	fmt.Println(Equals(a, b), Equals(a, nil))
	fmt.Println(Keys(map[string]bool{"x": true}))
	fmt.Println(Apply(func(i int) string { return fmt.Sprint(i * 2) }, 1, 2, 3))
}

// Output:
// true false
// 1
// [2 4 6]
