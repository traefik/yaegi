package main

type small[K comparable, V any] struct {
	ghost *ghost[K, V]
	n     int
}

func (s *small[K, V]) length() int { return s.n }

type ghost[K comparable, V any] struct {
	small *small[K, V]
}

func (g *ghost[K, V]) size() int { return g.small.length() }

func main() {
	s := &small[string, int]{n: 2}
	s.ghost = &ghost[string, int]{small: s}
	println(s.ghost.size())
}

// Output:
// 2
