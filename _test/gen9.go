package main

type Float interface {
	~float32 | ~float64
}

func add[T Float](a, b T) float64 { return float64(a) + float64(b) }

func main() {
	println(add(1, 2))
}

// Error:
// untyped int does not implement main.Float
