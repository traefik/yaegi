package main

import "fmt"

type Node[K comparable, V any] interface{ Key() K }

type b[K comparable, V any] struct{ key K }

func (n *b[K, V]) Key() K { return n.key }

func NewB[K comparable, V any](key K, value V) Node[K, V] { return &b[K, V]{key: key} }

type Manager[K comparable, V any] struct {
	create func(key K, value V) Node[K, V]
}

func NewManager[K comparable, V any]() *Manager[K, V] {
	m := &Manager[K, V]{}
	m.create = NewB[K, V]
	return m
}

func main() {
	f := NewB[string, int]
	fmt.Println(f("a", 1).Key(), NewManager[string, int]().create("b", 2).Key())
}

// Output:
// a b
