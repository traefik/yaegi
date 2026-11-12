// Zero values of type parameters (var zero T, *new(T), T{} forms) for every
// kind of type argument, comparison with the zero value for comparable T,
// and generic functions returning zero values on failure.
package main

import (
	"errors"
	"fmt"
)

type S struct {
	A int
	B string
	C []int
}

func Zero[T any]() T {
	var z T
	return z
}

func NewZero[T any]() T { return *new(T) }

func IsZero[T comparable](v T) bool {
	var z T
	return v == z
}

func Get[K comparable, V any](m map[K]V, k K) (V, error) {
	if v, ok := m[k]; ok {
		return v, nil
	}
	var zero V
	return zero, errors.New("missing key " + fmt.Sprint(k))
}

func Reset[T any](p *T) { *p = *new(T) }

func Describe[T any]() string {
	z := Zero[T]()
	return fmt.Sprintf("%T=%v", z, z)
}

func main() {
	fmt.Println(Describe[int](), Describe[string](), Describe[bool](), Describe[float32]())
	fmt.Println(Describe[S](), Describe[*S](), Describe[[]int](), Describe[map[string]int]())
	fmt.Println(Describe[[2]bool](), Describe[complex128](), Describe[any](), Describe[error]())
	fmt.Println(Describe[func()]() == "func()=<nil>", Describe[chan int]())
	fmt.Printf("%q %q\n", Zero[string](), NewZero[string]())
	fmt.Println(Zero[[]int]() == nil, NewZero[map[int]int]() == nil, Zero[*int]() == nil, Zero[any]() == nil, Zero[error]() == nil)
	fmt.Printf("%+v\n", NewZero[S]())

	fmt.Println(IsZero(0), IsZero(1), IsZero(""), IsZero("x"), IsZero(0.0), IsZero(false))
	fmt.Println(IsZero([2]int{}), IsZero([2]int{0, 1}), IsZero(struct{ A, B int }{}), IsZero[any](nil), IsZero[any](0))
	var ip *int
	fmt.Println(IsZero(ip), IsZero(&struct{}{}) == false)

	m := map[string]S{"a": {A: 1}}
	v, err := Get(m, "a")
	fmt.Println(v.A, err)
	v, err = Get(m, "b")
	fmt.Printf("%+v %v\n", v, err)
	n, err := Get(map[int]float64{}, 3)
	fmt.Println(n, err)

	x := S{1, "s", []int{1}}
	Reset(&x)
	fmt.Printf("%+v %v\n", x, x.C == nil)
	k := 42
	Reset(&k)
	fmt.Println(k)
}
