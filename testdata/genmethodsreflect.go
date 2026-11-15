//go:build go1.27

// reflect does not list the generic methods of Go 1.27 (they are not in
// the run-time method set): NumMethod counted them, and Method(i) and
// MethodByName gave a method without code (a nil panic when called).
package main

import (
	"fmt"
	"reflect"
)

type Box[T any] struct{ v T }

func (b Box[T]) Map[U any](f func(T) U) Box[U] { return Box[U]{f(b.v)} }
func (b Box[T]) String() string                { return fmt.Sprintf("Box(%v)", b.v) }
func (b *Box[T]) Set[U any](u U)               { fmt.Println("set", u) }
func (b *Box[T]) Put(v T)                      { b.v = v }

type E struct{ Box[int] }

type Plain struct{ n int }

func (p Plain) Twice[T any](t T) [2]T { return [2]T{t, t} }
func (p Plain) N() int                { return p.n }

func show(x any) {
	t := reflect.TypeOf(x)
	v := reflect.ValueOf(x)
	fmt.Println(t, "NumMethod", t.NumMethod(), v.NumMethod())
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		fmt.Println("  ", i, m.Name, m.Type)
	}
	if t.NumMethod() > 0 {
		out := v.Method(0).Type()
		fmt.Println("  method 0 type", out)
	}
	for _, name := range []string{"Map", "Set", "Twice", "String", "Put", "N"} {
		_, ok := t.MethodByName(name)
		fmt.Println("  MethodByName", name, ok, v.MethodByName(name).IsValid())
	}
}

func main() {
	b := Box[int]{3}
	fmt.Println(b, b.Map(func(i int) string { return fmt.Sprint(i * 2) }))
	show(b)
	show(&b)
	show(E{b})
	show(&E{b})
	show(Plain{4})
	show(&Plain{4})
	fmt.Println(reflect.ValueOf(b).MethodByName("String").Call(nil)[0])
	fmt.Println(reflect.ValueOf(Plain{5}).Method(0).Call(nil)[0])
	var s fmt.Stringer = b
	fmt.Println(s.String())
	_, ok := any(Plain{}).(interface{ N() int })
	fmt.Println(ok)
	p := &b
	p.Set[string]("x")
	p.Put(9)
	fmt.Println(Plain{1}.Twice[string]("a"), b)
}
