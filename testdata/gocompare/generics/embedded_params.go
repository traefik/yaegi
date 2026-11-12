// Generic structs embedding generic types and other instances, promoted
// fields and methods through generic embedding, generic interfaces
// embedded in interfaces, and fields whose type is a type parameter.
package main

import "fmt"

type Base[T any] struct {
	Val T
}

func (b Base[T]) Get() T       { return b.Val }
func (b *Base[T]) Set(v T)     { b.Val = v }
func (b Base[T]) Desc() string { return fmt.Sprintf("Base[%T](%v)", b.Val, b.Val) }

type Labeled[T any] struct {
	Base[T]
	Label string
}

func (l Labeled[T]) Desc() string { return l.Label + ":" + l.Base.Desc() }

type Pointed[T any] struct {
	*Base[T]
	N int
}

type Both[K comparable, V any] struct {
	Base[K]
	Other Base[V]
	M     map[K]V
}

type Getter[T any] interface{ Get() T }
type GetSetter[T any] interface {
	Getter[T]
	Set(T)
}

type Wrap[T any] struct {
	Inner T
}

type Meta struct{ Version int }

func (m Meta) Ver() string { return fmt.Sprint("v", m.Version) }

type Doc[T any] struct {
	Meta
	Body T
}

func use[T any](gs GetSetter[T], v T) T {
	gs.Set(v)
	return gs.Get()
}

func main() {
	l := Labeled[int]{Base[int]{3}, "n"}
	fmt.Println(l.Val, l.Get(), l.Desc(), l.Base.Desc())
	l.Set(4)
	fmt.Println(l.Val, l)
	fmt.Printf("%+v\n", l)

	p := Pointed[string]{&Base[string]{"s"}, 1}
	p.Set("t")
	fmt.Println(p.Get(), p.Val, p.Desc())

	bo := Both[string, []int]{Base[string]{"k"}, Base[[]int]{[]int{1}}, map[string][]int{}}
	bo.M[bo.Get()] = bo.Other.Get()
	fmt.Println(bo.M, bo.Desc(), bo.Other.Desc())

	var g Getter[int] = l
	fmt.Println(g.Get())
	fmt.Println(use[int](&l, 9), l.Val)
	fmt.Println(use[string](p, "u"), p.Val)
	_, ok := any(l).(GetSetter[int])
	_, ok2 := any(&l).(GetSetter[int])
	fmt.Println(ok, ok2)

	w := Wrap[Wrap[Base[bool]]]{}
	w.Inner.Inner.Val = true
	fmt.Println(w, w.Inner.Inner.Get())

	d := Doc[[]string]{Meta{2}, []string{"x"}}
	fmt.Println(d.Ver(), d.Version, d.Body)
	fmt.Printf("%+v\n", d)
}
