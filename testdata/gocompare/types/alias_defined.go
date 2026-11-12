// Defined types versus aliases: type A = B shares methods and identity,
// type D B is a new type without B's methods; conversions between them,
// %T of each, and assignability through interfaces and assertions.
package main

import "fmt"

type Base struct{ N int }

func (b Base) Hello() string { return fmt.Sprint("hello ", b.N) }

type Alias = Base
type Defined Base
type DefinedAlias = Defined
type IntAlias = int
type MyInt int
type MyIntAlias = MyInt
type Slice = []string
type Fn = func(int) int
type M = map[string]int

func (d Defined) Bye() string { return fmt.Sprint("bye ", d.N) }

type Greeter interface{ Hello() string }

func main() {
	var a Alias = Base{1}
	var b Base = a // same type
	fmt.Println(a.Hello(), b.Hello())
	fmt.Printf("%T %T\n", a, b)

	d := Defined{2}
	fmt.Println(d.Bye(), d.N)
	fmt.Printf("%T\n", d)
	b2 := Base(d)
	d2 := Defined(b2)
	fmt.Println(b2.Hello(), d2.Bye())
	var da DefinedAlias = d
	fmt.Printf("%T %s\n", da, da.Bye())

	var g Greeter = a
	_, isBase := g.(Base)
	_, isAlias := g.(Alias)
	_, isDef := any(d).(Greeter)
	fmt.Println(isBase, isAlias, isDef)

	var ia IntAlias = 5
	var i int = ia
	fmt.Printf("%T %v\n", ia, i+ia)
	var mi MyIntAlias = 3
	var mi2 MyInt = mi
	fmt.Printf("%T %v\n", mi, mi2*2)
	_, isInt := any(mi).(int)
	_, isMyInt := any(mi).(MyInt)
	fmt.Println(isInt, isMyInt)

	var s Slice = []string{"a"}
	s = append(s, "b")
	var raw []string = s
	fmt.Printf("%T %v %v\n", s, s, raw)
	var f Fn = func(x int) int { return x * 3 }
	fmt.Printf("%T %v\n", f, f(2))
	var m M = map[string]int{"a": 1}
	fmt.Printf("%T %v\n", m, m)

	// Defined types of composite types.
	type Names []string
	type Table map[string][]int
	n := Names{"x", "y"}
	fmt.Printf("%T %v %d\n", n, n, len(n))
	var plain []string = []string(n)
	n2 := Names(plain)
	fmt.Println(len(plain), n2[1])
	t := Table{"k": {1, 2}}
	t["k"] = append(t["k"], 3)
	fmt.Printf("%T %v\n", t, t)

	// Untyped constants and named types.
	const c = 10
	var x MyInt = c
	var y Defined = Defined{N: c}
	fmt.Println(x, y)

	// Local alias and local defined type in a function.
	type L = struct{ V int }
	type LD struct{ V int }
	l := L{1}
	ld := LD(l)
	fmt.Printf("%T %T %v %v\n", l, ld, l, ld)
	var anon struct{ V int } = l
	fmt.Println(anon == l)
}
