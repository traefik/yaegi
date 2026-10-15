package main

type M[K comparable, V any] struct{ m map[K]V }

func (m *M[K, V]) Set(k K, v V) bool { return m.set(k, v, false) }

func (m *M[K, V]) SetIfAbsent(k K, v V) bool { return m.set(k, v, true) }

func (m *M[K, V]) set(k K, v V, onlyIfAbsent bool) bool {
	if _, ok := m.m[k]; ok && onlyIfAbsent {
		return false
	}
	m.m[k] = v
	return true
}

func main() {
	m := &M[string, int]{m: map[string]int{}}
	println(m.Set("a", 1), m.SetIfAbsent("a", 2), m.m["a"])
}

// Output:
// true false 1
