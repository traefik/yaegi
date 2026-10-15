package main

type Item[K comparable, V any] struct {
	key   K
	value V
}

type Loader[K comparable, V any] interface {
	Load(c *Cache[K, V], key K) *Item[K, V]
}

type options[K comparable, V any] struct {
	loader Loader[K, V]
}

type Cache[K comparable, V any] struct {
	items   map[K]*Item[K, V]
	options options[K, V]
}

func New[K comparable, V any]() *Cache[K, V] {
	return &Cache[K, V]{items: map[K]*Item[K, V]{}}
}

func (c *Cache[K, V]) Get(k K) V {
	if it, ok := c.items[k]; ok {
		return it.value
	}
	if c.options.loader != nil {
		return c.options.loader.Load(c, k).value
	}
	var zero V
	return zero
}

type loader struct{}

func (loader) Load(c *Cache[string, string], key string) *Item[string, string] {
	it := &Item[string, string]{key: key, value: "loaded-" + key}
	c.items[key] = it
	return it
}

func main() {
	c := New[string, string]()
	var l Loader[string, string] = loader{}
	c.options.loader = l
	println(c.Get("a"), c.Get("a"), len(c.items))
}

// Output:
// loaded-a loaded-a 1
