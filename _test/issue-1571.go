package main

type A struct {
	*B[string]
}

type B[T any] struct {
	data T
}

func main() {
	_ = &A{
		B: &B[string]{},
	}

	println("PASS")
}

// Output:
// PASS
