package main

// reflect names of types declared inside functions, as gc gives them: the
// type arguments of the enclosing generic function ("L[int]",
// "M[int;string]"), and the "·N" number of local types inside type
// arguments (GOROOT/test typeparam/nested.go, typeparam/issue54456.go).

import (
	"fmt"
	"reflect"
)

type G[T any] struct{}

func F[A any]() {
	type L int
	type M[B any] struct{}
	type N struct{ x A }
	fmt.Println(reflect.TypeOf(L(0)), reflect.TypeOf(M[int]{}), reflect.TypeOf(N{}))
	fmt.Println(reflect.TypeOf(G[L]{}), reflect.TypeOf(G[M[L]]{}), reflect.TypeOf(M[M[L]]{}))
	fmt.Println(reflect.TypeOf([]L{}), reflect.TypeOf(new(L)).Elem().Name(), reflect.TypeOf(M[int]{}).Name())
	fmt.Println(reflect.TypeOf(func(L) M[A] { return M[A]{} }).String())
}

func H() {
	type L int
	type P struct{}
	{
		type Q int
		fmt.Println(reflect.TypeOf(G[Q]{}), reflect.TypeOf(Q(0)))
	}
	fmt.Println(reflect.TypeOf(L(0)), reflect.TypeOf(G[L]{}), reflect.TypeOf(G[P]{}).String())
}

func one() any { type T[_ any] int; return T[int](0) }
func two() any { type T[_ any] int; return T[int](0) }

func main() {
	type L int
	F[int]()
	F[L]()
	F[G[string]]()
	H()
	fmt.Println(reflect.TypeOf(G[L]{}))
	func() {
		type K int
		fmt.Println(reflect.TypeOf(G[K]{}))
	}()
	p, q := one(), two()
	fmt.Println(p == q, reflect.TypeOf(p), reflect.TypeOf(q), reflect.TypeOf(p) == reflect.TypeOf(q))
}
