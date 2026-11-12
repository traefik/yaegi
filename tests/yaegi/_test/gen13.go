package main

type Map[K comparable, V any] struct {
	ж map[K]V
}

func (m Map[K, V]) Has(k K) bool {
	_, ok := m.ж[k]
	return ok
}

func main() {
	m := Map[string, float64]{}
	println(m.Has("test"))
}

// Output:
// false
