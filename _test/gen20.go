package main

type A[T any] struct{ v T }

type B[T any] struct{ v T }

func (a *A[T]) Name() string { return "A" }

func (b *B[T]) Name() string { return "B" }

func (a *A[T]) Self() *A[T] { return a }

func (b *B[T]) Self() *B[T] { return b }

func main() {
	a := &A[int]{}
	b := &B[int]{}
	println(a.Name(), b.Name(), a.Self().Name(), b.Self().Name())
}

// Output:
// A B A B
