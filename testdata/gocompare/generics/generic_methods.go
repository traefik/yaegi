// Methods of generic types: value and pointer receivers, method values and
// method expressions of instances, generic types satisfying interfaces
// (generic and not), a generic Stringer, and constraints with methods
// used on generic receivers.
package main

import (
	"fmt"
	"sort"
	"strings"
)

type Stringer interface{ String() string }

type Container[T any] interface {
	Add(T)
	All() []T
	Len() int
}

type Bag[T any] struct{ items []T }

func (b *Bag[T]) Add(v T)  { b.items = append(b.items, v) }
func (b *Bag[T]) All() []T { return b.items }
func (b Bag[T]) Len() int  { return len(b.items) }
func (b Bag[T]) String() string {
	parts := make([]string, len(b.items))
	for i, v := range b.items {
		parts[i] = fmt.Sprint(v)
	}
	return "Bag{" + strings.Join(parts, ",") + "}"
}

type Named[T fmt.Stringer] struct {
	vals []T
}

func (n Named[T]) Join(sep string) string {
	var out []string
	for _, v := range n.vals {
		out = append(out, v.String())
	}
	return strings.Join(out, sep)
}

type Color int

func (c Color) String() string { return [...]string{"red", "green", "blue"}[c] }

type Lesser[T any] interface {
	Less(T) bool
}

type Ver struct{ Major, Minor int }

func (v Ver) Less(o Ver) bool {
	if v.Major != o.Major {
		return v.Major < o.Major
	}
	return v.Minor < o.Minor
}

func SortL[T Lesser[T]](xs []T) {
	sort.Slice(xs, func(i, j int) bool { return xs[i].Less(xs[j]) })
}

// PtrTo constrains *T to have a method: the classic pointer-method constraint.
type Setter[T any] interface {
	*T
	Set(string)
}

type Field struct{ v string }

func (f *Field) Set(s string) { f.v = "set:" + s }

func FromStrings[T any, PT Setter[T]](ss []string) []T {
	out := make([]T, len(ss))
	for i, s := range ss {
		PT(&out[i]).Set(s)
	}
	return out
}

type Num[T ~int | ~float64] struct{ v T }

func (n Num[T]) Add(o Num[T]) Num[T] { return Num[T]{n.v + o.v} }
func (n *Num[T]) Inc()               { n.v++ }

func fill[T any](c Container[T], xs ...T) Container[T] {
	for _, x := range xs {
		c.Add(x)
	}
	return c
}

func main() {
	b := &Bag[int]{}
	b.Add(1)
	b.Add(2)
	fmt.Println(b.All(), b.Len(), b, *b)
	var c Container[string] = &Bag[string]{}
	fill(c, "x", "y")
	fmt.Println(c.All(), c.Len(), c)
	var st Stringer = Bag[float64]{items: []float64{1.5}}
	fmt.Println(st.String())
	_, ok := any(Bag[int]{}).(Container[int])
	_, ok2 := any(&Bag[int]{}).(Container[int])
	_, ok3 := any(&Bag[int]{}).(Container[string])
	fmt.Println(ok, ok2, ok3)

	n := Named[Color]{[]Color{0, 2, 1}}
	fmt.Println(n.Join("|"))
	fmt.Println(Color(1), []Color{2, 0})

	vs := []Ver{{1, 2}, {0, 9}, {1, 0}}
	SortL(vs)
	fmt.Println(vs)

	fs := FromStrings[Field]([]string{"a", "b"})
	fmt.Println(fs)

	// Method values and expressions of instances.
	x := Num[int]{1}
	add := x.Add
	inc := x.Inc
	inc()
	inc()
	fmt.Println(add(Num[int]{10}), x)
	addExpr := Num[float64].Add
	incExpr := (*Num[float64]).Inc
	y := Num[float64]{0.5}
	incExpr(&y)
	fmt.Println(addExpr(y, Num[float64]{0.25}))
	bl := (*Bag[string]).Len
	fmt.Println(bl(&Bag[string]{items: []string{"q"}}))
}
