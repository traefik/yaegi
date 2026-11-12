// Generic type aliases (type A[T any] = ...), aliases of instances, and
// aliases used in signatures, conversions, %T and method calls.
package main

import (
	"fmt"
	"iter"
	"slices"
)

type Pair[K comparable, V any] struct {
	K K
	V V
}

func (p Pair[K, V]) Flip() Pair[K, V] { return p }
func (p Pair[K, V]) String() string   { return fmt.Sprintf("(%v,%v)", p.K, p.V) }

type Set[T comparable] = map[T]struct{}
type StrPair[V any] = Pair[string, V]
type IntPair = Pair[int, int]
type Seq[T any] = iter.Seq[T]
type List[T any] = []T
type Fn[A, B any] = func(A) B

func Of[T comparable](xs ...T) Set[T] {
	s := Set[T]{}
	for _, x := range xs {
		s[x] = struct{}{}
	}
	return s
}

func Each[T any](xs List[T]) Seq[T] {
	return func(yield func(T) bool) {
		for _, x := range xs {
			if !yield(x) {
				return
			}
		}
	}
}

func Compose[A, B, C any](f Fn[A, B], g Fn[B, C]) Fn[A, C] {
	return func(a A) C { return g(f(a)) }
}

func main() {
	s := Of(3, 1, 3, 2)
	var plain map[int]struct{} = s
	fmt.Println(len(s), len(plain), s)
	fmt.Printf("%T\n", s)

	sp := StrPair[int]{"a", 1}
	var p Pair[string, int] = sp
	fmt.Println(sp, p, sp.Flip())
	fmt.Printf("%T %T\n", sp, p)
	ip := IntPair{1, 2}
	fmt.Printf("%v %T\n", ip, ip)
	_, isPair := any(ip).(Pair[int, int])
	fmt.Println(isPair)

	fmt.Println(slices.Collect(Each(List[string]{"x", "y"})))
	f := Compose(func(i int) string { return fmt.Sprint(i * 2) }, func(s string) int { return len(s) })
	fmt.Println(f(5), f(50))
	fmt.Printf("%T\n", f)
	var lst List[IntPair] = []Pair[int, int]{{1, 1}}
	fmt.Println(lst)
}
