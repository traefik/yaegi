// Struct comparison with == and != (nested structs, arrays, strings,
// floats with NaN, pointers, interfaces), structs as map keys, and the
// panic when comparing structs that hold uncomparable values in interfaces.
package main

import (
	"fmt"
	"math"
)

type P struct {
	X, Y int
}

type K struct {
	Name string
	P    P
	A    [2]int8
}

type F struct {
	V float64
}

type WithPtr struct {
	P *int
}

type WithIface struct {
	V any
}

type Blank struct {
	A int
	_ int
	B string
}

func main() {
	fmt.Println(P{1, 2} == P{1, 2}, P{1, 2} == P{2, 1}, P{} != P{0, 0})
	k1 := K{"a", P{1, 2}, [2]int8{3, 4}}
	k2 := k1
	fmt.Println(k1 == k2)
	k2.A[1] = 5
	fmt.Println(k1 == k2, k1 != k2)
	k2 = k1
	k2.P.Y = 0
	fmt.Println(k1 == k2)

	nan := math.NaN()
	fmt.Println(F{nan} == F{nan}, F{0} == F{math.Copysign(0, -1)}, F{1.5} == F{1.5})

	x, y := 1, 1
	fmt.Println(WithPtr{&x} == WithPtr{&x}, WithPtr{&x} == WithPtr{&y}, WithPtr{} == WithPtr{nil})

	fmt.Println(WithIface{1} == WithIface{1}, WithIface{1} == WithIface{int64(1)}, WithIface{"s"} == WithIface{"s"}, WithIface{} == WithIface{nil})
	fmt.Println(WithIface{P{1, 2}} == WithIface{P{1, 2}}, WithIface{[2]int{1, 2}} == WithIface{[2]int{1, 3}})

	cmp := func(a, b WithIface) (res bool, err any) {
		defer func() { err = recover() }()
		return a == b, nil
	}
	fmt.Println(cmp(WithIface{[]int{1}}, WithIface{[]int{1}}))
	fmt.Println(cmp(WithIface{map[string]int{}}, WithIface{1}))
	fmt.Println(cmp(WithIface{1}, WithIface{[]int{1}}))
	fmt.Println(cmp(WithIface{func() {}}, WithIface{func() {}}))
	fmt.Println(cmp(WithIface{struct{ S []int }{}}, WithIface{struct{ S []int }{}}))

	fmt.Println(Blank{A: 1, B: "x"} == Blank{A: 1, B: "x"})

	// Structs as map keys.
	m := map[K]int{}
	m[k1]++
	m[k1]++
	m[K{Name: "b"}] = 7
	fmt.Println(len(m), m[k1], m[K{Name: "b"}], m[K{}])
	pm := map[P]string{{1, 2}: "a", {2, 1}: "b"}
	fmt.Println(pm[P{1, 2}], pm[P{2, 1}], pm)
	im := map[any]int{P{1, 1}: 1, K{}: 2, "s": 3}
	fmt.Println(im[P{1, 1}], im[K{}], im["s"], im[P{}])
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		im[[]int{1}] = 4
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		_ = im[WithIface{map[int]int{}}]
	}()

	// Anonymous struct comparison with a named struct of the same fields.
	an := struct{ X, Y int }{1, 2}
	fmt.Println(an == P{1, 2}, P(an) == P{1, 2})

	// Arrays of structs.
	a1 := [2]P{{1, 2}, {3, 4}}
	a2 := [2]P{{1, 2}, {3, 4}}
	fmt.Println(a1 == a2)
	a2[1].Y = 0
	fmt.Println(a1 == a2)
}
