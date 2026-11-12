// %T, %v and reflect type names of generic instances: main.Pair[int,string],
// nested instances, instances with named, pointer, slice, map, func and
// struct type arguments, local generic types, and reflect Name/String.
package main

import (
	"fmt"
	"reflect"
)

type Pair[K comparable, V any] struct {
	K K
	V V
}

type Box[T any] struct{ V T }

type List[T any] []T

type Fn[T any] func(T) T

type Num[T ~int | ~float64] struct{ V T }

type MyInt int

type Wrapper[T any] interface{ Unwrap() T }

func (b Box[T]) Unwrap() T { return b.V }

func typeName[T any]() string {
	var p *T
	return reflect.TypeOf(p).Elem().String()
}

func main() {
	fmt.Printf("%T\n", Pair[int, string]{})
	fmt.Printf("%T\n", &Pair[string, []int]{})
	fmt.Printf("%T\n", Box[Pair[int, bool]]{})
	fmt.Printf("%T\n", Box[*MyInt]{})
	fmt.Printf("%T\n", Box[map[string][]MyInt]{})
	fmt.Printf("%T\n", Box[func(int) error]{})
	fmt.Printf("%T\n", Box[struct{ A int }]{})
	fmt.Printf("%T\n", Box[[3]byte]{})
	fmt.Printf("%T\n", Box[any]{})
	fmt.Printf("%T\n", Box[error]{})
	fmt.Printf("%T\n", Box[chan<- int]{})
	fmt.Printf("%T\n", List[MyInt]{1})
	fmt.Printf("%T\n", Fn[string](nil))
	fmt.Printf("%T\n", Num[float64]{1.5})
	fmt.Printf("%T\n", []Box[int]{})
	fmt.Printf("%T\n", map[Pair[int, int]]Box[string]{})
	fmt.Printf("%T\n", Box[Box[Box[uint8]]]{})

	fmt.Printf("%v %+v\n", Pair[int, string]{1, "a"}, Pair[int, string]{1, "a"})
	fmt.Printf("%#v\n", Box[int]{3})
	fmt.Printf("%v %v\n", List[int]{1, 2}, Num[int]{5})

	var w Wrapper[int] = Box[int]{9}
	fmt.Printf("%T %v\n", w, w.Unwrap())

	for _, t := range []reflect.Type{
		reflect.TypeOf(Pair[int, string]{}),
		reflect.TypeOf(Box[MyInt]{}),
		reflect.TypeOf(List[string]{}),
		reflect.TypeOf((*Wrapper[bool])(nil)).Elem(),
	} {
		fmt.Printf("%s | %s | %v\n", t.String(), t.Name(), t.Kind())
	}
	fmt.Println(typeName[int](), typeName[Pair[MyInt, []string]](), typeName[Box[any]]())

	type Local[T any] struct{ X T }
	fmt.Printf("%T %v\n", Local[int]{1}, Local[string]{"s"})
}
