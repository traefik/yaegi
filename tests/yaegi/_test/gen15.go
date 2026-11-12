package main

import "unsafe"

type Node[K comparable, V any] struct {
	key   K
	value V
}

func (n *Node[K, V]) AsPointer() unsafe.Pointer { return unsafe.Pointer(n) }

func CastFromPointer[K comparable, V any](ptr unsafe.Pointer) *Node[K, V] {
	return (*Node[K, V])(ptr)
}

type Pair[K comparable, V any] struct {
	key   K
	value V
}

type MyPair[K comparable, V any] Pair[K, V]

func convert[K comparable, V any](p MyPair[K, V]) Pair[K, V] { return Pair[K, V](p) }

func assert[K comparable, V any](i any) (Pair[K, V], bool) {
	p, ok := i.(Pair[K, V])
	return p, ok
}

func main() {
	n := &Node[string, int]{key: "a", value: 1}
	println(CastFromPointer[string, int](n.AsPointer()).value)

	c := convert[string, int](MyPair[string, int]{"b", 2})
	println(c.key, c.value)

	p, ok := assert[string, int](Pair[string, int]{"c", 3})
	println(p.key, p.value, ok)
}

// Output:
// 1
// b 2
// c 3 true
