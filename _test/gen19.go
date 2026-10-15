package main

type T[K comparable, V any] struct {
	k K
}

func New[K comparable, V any](k K) *T[K, V] {
	return &T[K, V]{unknown: k}
}

func main() {
	t := New[string, int]("a")
	println(t.k)
}

// Error:
// _test/gen19.go:8:18: unknown field unknown in struct literal
