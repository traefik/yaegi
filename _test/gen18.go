package main

type options[K comparable, V any] struct {
	capacity int
}

type Builder[K comparable, V any] struct {
	options[K, V]
}

func NewBuilder[K comparable, V any](capacity int) *Builder[K, V] {
	return &Builder[K, V]{options: options[K, V]{capacity: capacity}}
}

func (b *Builder[K, V]) Self() *Builder[K, V] { return b }

func main() {
	b := NewBuilder[string, int](10)
	println(b.capacity, b.options.capacity, NewBuilder[string, int](20).Self().capacity)
}

// Output:
// 10 10 20
