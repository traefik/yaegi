package main

import "unsafe"

type table[K comparable] struct{ size int }

type Map[K comparable, V any] struct{ t unsafe.Pointer }

func NewMap[K comparable, V any](size int) *Map[K, V] {
	return &Map[K, V]{t: unsafe.Pointer(&table[K]{size: size})}
}

func (m *Map[K, V]) Size() int {
	t := (*table[K])(m.t)
	return t.size
}

func main() {
	println(NewMap[string, int](3).Size())
}

// Output:
// 3
