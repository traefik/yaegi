package main

import "fmt"

func zeroValue[V any]() V {
	var zero V
	return zero
}

type Cache[K comparable, V any] struct{ m map[K]V }

func (c *Cache[K, V]) Get(k K) (V, bool) {
	v, ok := c.m[k]
	if !ok {
		return zeroValue[V](), false
	}
	return v, true
}

func main() {
	c := &Cache[string, []byte]{m: map[string][]byte{"a": []byte("x")}}
	fmt.Println(c.Get("a"))
	fmt.Println(c.Get("b"))
}

// Output:
// [120] true
// [] false
