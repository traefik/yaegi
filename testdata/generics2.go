// Generics beyond the basics: constraints with methods and type sets,
// generic types with methods, generic types that embed, recursive generic
// types, instantiation with interpreted and native types, type inference,
// generic closures, type switches on type parameters, zero values and
// generic iterators.
package main

import (
	"cmp"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Stringer interface{ String() string }

type Celsius float64

func (c Celsius) String() string { return strconv.FormatFloat(float64(c), 'f', 1, 64) + "C" }

type ID int

func (i ID) String() string { return "#" + strconv.Itoa(int(i)) }

// A constraint with a method and a type set.
type StringNum interface {
	~int | ~float64
	String() string
}

func Describe[T StringNum](xs []T) string {
	var total T
	var parts []string
	for _, x := range xs {
		total += x
		parts = append(parts, x.String())
	}
	return strings.Join(parts, "+") + "=" + total.String()
}

// Generic type with methods and a pointer receiver.
type Stack[T any] struct{ items []T }

func (s *Stack[T]) Push(v T) { s.items = append(s.items, v) }
func (s *Stack[T]) Pop() (T, bool) {
	var zero T
	if len(s.items) == 0 {
		return zero, false
	}
	v := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return v, true
}
func (s Stack[T]) Len() int { return len(s.items) }

// Generic type that embeds another generic type.
type Named[T any] struct {
	Stack[T]
	Name string
}

// Recursive generic type.
type Tree[K cmp.Ordered, V any] struct {
	root *node[K, V]
	size int
}

type node[K cmp.Ordered, V any] struct {
	k           K
	v           V
	left, right *node[K, V]
}

func (t *Tree[K, V]) Put(k K, v V) {
	p := &t.root
	for *p != nil {
		switch c := cmp.Compare(k, (*p).k); {
		case c < 0:
			p = &(*p).left
		case c > 0:
			p = &(*p).right
		default:
			(*p).v = v
			return
		}
	}
	*p = &node[K, V]{k: k, v: v}
	t.size++
}

func (t *Tree[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		var walk func(n *node[K, V]) bool
		walk = func(n *node[K, V]) bool {
			if n == nil {
				return true
			}
			return walk(n.left) && yield(n.k, n.v) && walk(n.right)
		}
		walk(t.root)
	}
}

// Type switch on a type parameter value.
func Kind[T any](v T) string {
	switch x := any(v).(type) {
	case int:
		return "int " + strconv.Itoa(x)
	case string:
		return "string " + x
	case Stringer:
		return "Stringer " + x.String()
	case nil:
		return "nil"
	default:
		return fmt.Sprintf("other %T", x)
	}
}

func Zero[T any]() T {
	var z T
	return z
}

// Map, Filter, Reduce with inference.
func Map[T, U any](xs []T, f func(T) U) []U {
	out := make([]U, 0, len(xs))
	for _, x := range xs {
		out = append(out, f(x))
	}
	return out
}

func Filter[S ~[]E, E any](s S, keep func(E) bool) S {
	var out S
	for _, e := range s {
		if keep(e) {
			out = append(out, e)
		}
	}
	return out
}

func Reduce[T, A any](xs []T, init A, f func(A, T) A) A {
	acc := init
	for _, x := range xs {
		acc = f(acc, x)
	}
	return acc
}

type IDs []ID

func (s IDs) Total() int {
	n := 0
	for _, x := range s {
		n += int(x)
	}
	return n
}

// Generic constraint with a pointer method (the *T pattern).
type Setter[T any] interface {
	*T
	Set(string) error
}

type Port struct{ N int }

func (p *Port) Set(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	if n <= 0 || n > 65535 {
		return errors.New("bad port")
	}
	p.N = n
	return nil
}

func ParseAll[T any, PT Setter[T]](ss []string) ([]T, error) {
	out := make([]T, len(ss))
	for i, s := range ss {
		if err := PT(&out[i]).Set(s); err != nil {
			return nil, fmt.Errorf("item %d: %w", i, err)
		}
	}
	return out, nil
}

// Generic func values and closures.
func Memo[K comparable, V any](f func(K) V) func(K) V {
	cache := map[K]V{}
	return func(k K) V {
		if v, ok := cache[k]; ok {
			return v
		}
		v := f(k)
		cache[k] = v
		return v
	}
}

type Pair[A, B any] struct {
	First  A
	Second B
}

func (p Pair[A, B]) Swap() Pair[B, A] { return Pair[B, A]{p.Second, p.First} }
func (p Pair[A, B]) String() string   { return fmt.Sprintf("(%v, %v)", p.First, p.Second) }

type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~float32 | ~float64
}

func SumAll[T Number](xs ...T) T {
	var s T
	for _, x := range xs {
		s += x
	}
	return s
}

// Generic interface.
type Container[T any] interface {
	Put(T)
	Items() []T
}

type Bag[T any] struct{ xs []T }

func (b *Bag[T]) Put(v T)    { b.xs = append(b.xs, v) }
func (b *Bag[T]) Items() []T { return b.xs }

func Fill[T any](c Container[T], vs ...T) Container[T] {
	for _, v := range vs {
		c.Put(v)
	}
	return c
}

// Generic alias (Go 1.24).
type Set[T comparable] = map[T]struct{}

func Keys[T cmp.Ordered](s Set[T]) []T {
	return slices.Sorted(maps.Keys(s))
}

func main() {
	fmt.Println(Describe([]Celsius{1.5, 2.25}))
	fmt.Println(Describe([]ID{1, 2, 3}))

	var s Stack[string]
	s.Push("a")
	s.Push("b")
	v, ok := s.Pop()
	fmt.Println(v, ok, s.Len())
	var n Named[time.Duration]
	n.Name = "durations"
	n.Push(time.Second)
	n.Push(2 * time.Millisecond)
	d, _ := n.Pop()
	fmt.Println(n.Name, d, n.Len())
	_, ok = (&Stack[int]{}).Pop()
	fmt.Println(ok)

	var t Tree[string, int]
	for i, w := range strings.Fields("the quick brown fox jumps over the lazy dog") {
		t.Put(w, i)
	}
	for k, v := range t.All() {
		fmt.Print(k, ":", v, " ")
	}
	fmt.Println(t.size)
	for k := range t.All() {
		if k > "f" {
			fmt.Println("first after f:", k)
			break
		}
	}

	fmt.Println(Kind(3), Kind("x"), Kind(ID(4)), Kind[any](nil), Kind(3.5), Kind([]int{}))
	fmt.Printf("%q %v %v %v\n", Zero[string](), Zero[*int]() == nil, Zero[Pair[int, string]](), Zero[[2]bool]())

	strs := Map([]int{1, 2, 3}, strconv.Itoa)
	fmt.Println(strs, len(strs))
	ids := Filter(IDs{1, 2, 3, 4, 5, 6}, func(i ID) bool { return i%2 == 0 })
	fmt.Printf("%T %v %d\n", ids, ids, ids.Total())
	fmt.Println(Reduce([]string{"a", "b", "c"}, 0, func(n int, s string) int { return n + len(s) }))
	fmt.Println(Reduce(Map([]ID{5, 6}, ID.String), "", func(a, s string) string { return a + s }))

	ports, err := ParseAll[Port]([]string{"80", "443"})
	fmt.Println(ports, err)
	_, err = ParseAll[Port]([]string{"80", "0"})
	fmt.Println(err)

	calls := 0
	slow := Memo(func(n int) string { calls++; return strings.Repeat("x", n) })
	fmt.Println(slow(3), slow(3), slow(2), calls)

	p := Pair[string, int]{"a", 1}
	fmt.Println(p, p.Swap(), p.Swap().Swap() == p)
	var st fmt.Stringer = p.Swap()
	fmt.Println(st.String())

	fmt.Println(SumAll(1, 2, 3), SumAll[int8](100, 27), SumAll[uint8](200, 100), SumAll(0.5, 0.25))

	c := Fill[int](&Bag[int]{}, 3, 1, 2)
	items := c.Items()
	sort.Ints(items)
	fmt.Println(items)

	set := Set[string]{"z": {}, "a": {}, "m": {}}
	fmt.Println(Keys(set))

	fmt.Println(slices.MaxFunc([]Pair[string, int]{{"a", 3}, {"b", 9}, {"c", 1}}, func(x, y Pair[string, int]) int {
		return cmp.Compare(x.Second, y.Second)
	}))
	idx, found := slices.BinarySearchFunc([]ID{1, 3, 5, 7}, ID(5), func(a, b ID) int { return cmp.Compare(a, b) })
	fmt.Println(idx, found)
	fmt.Println(slices.Collect(maps.Values(map[ID]Celsius{1: 20})))
}
