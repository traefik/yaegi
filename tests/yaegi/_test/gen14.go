package main

import "container/list"

type Item[K comparable, V any] struct {
	key   K
	value V
}

type Cache[K comparable, V any] struct{ l *list.List }

func (c *Cache[K, V]) Set(k K, v V) { c.l.PushBack(&Item[K, V]{key: k, value: v}) }

func (c *Cache[K, V]) First() (V, bool) {
	if it, ok := c.l.Front().Value.(*Item[K, V]); ok {
		return it.value, true
	}
	var zero V
	return zero, false
}

func main() {
	c := &Cache[string, int]{l: list.New()}
	c.Set("a", 1)
	v, ok := c.First()
	println(v, ok)
}

// Output:
// 1 true
