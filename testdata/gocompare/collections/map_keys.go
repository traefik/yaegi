// Map key types: structs, arrays, nested structs, interfaces holding
// different dynamic types, booleans, floats (+0 and -0), runes, named
// types, channels compared by identity, and the panic of an unhashable
// interface key.
package main

import (
	"fmt"
	"sort"
)

type Pt struct{ X, Y int }

type Key struct {
	Name string
	P    Pt
	Tags [2]string
}

type Celsius float64

type Color int

func main() {
	sm := map[Pt]string{{1, 2}: "a", {2, 1}: "b"}
	sm[Pt{1, 2}] += "!"
	fmt.Println(sm, len(sm))

	km := map[Key]int{}
	km[Key{"a", Pt{1, 1}, [2]string{"x", "y"}}] = 1
	km[Key{"a", Pt{1, 1}, [2]string{"y", "x"}}] = 2
	km[Key{"a", Pt{1, 1}, [2]string{"x", "y"}}]++
	fmt.Println(len(km), km[Key{"a", Pt{1, 1}, [2]string{"x", "y"}}], km[Key{}])

	im := map[any]string{}
	im[1] = "int"
	im[int64(1)] = "int64"
	im[1.0] = "float64"
	im["1"] = "string"
	im[Pt{1, 1}] = "Pt"
	im[[2]int{1, 1}] = "array"
	im[true] = "bool"
	im[nil] = "nil"
	im[Color(1)] = "Color"
	fmt.Println(len(im), im[1], im[int64(1)], im[float64(1)], im[Pt{1, 1}], im[nil], im[Color(1)], im[int8(1)] == "")
	var keys []string
	for k, v := range im {
		keys = append(keys, fmt.Sprintf("%T=%v", k, v))
	}
	sort.Strings(keys)
	fmt.Println(keys)

	// Unhashable dynamic types panic.
	for _, k := range []any{[]int{1}, map[int]int{}, func() {}} {
		func() {
			defer func() { fmt.Println("recovered:", recover()) }()
			im[k] = "bad"
		}()
	}
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		_ = im[[]string{"x"}]
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		_ = im[[1]any{[]int{}}]
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		type S struct{ V any }
		m := map[S]int{}
		m[S{map[string]int{}}] = 1
	}()

	bm := map[bool]int{true: 1}
	bm[false]--
	fmt.Println(bm)

	fm := map[float64]string{}
	negZero := 0.0
	negZero = -negZero
	fm[0] = "zero"
	fm[negZero] = "neg zero"
	fm[0.1+0.2] = "sum"
	fm[0.3] = "lit"
	fmt.Println(len(fm), fm[0], fm)

	rm := map[rune]int{}
	for _, r := range "hello, wörld" {
		rm[r]++
	}
	fmt.Println(rm)

	cm := map[Celsius]Color{-40: 1, 100: 2}
	fmt.Println(cm[Celsius(-40)], cm)

	c1 := make(chan int)
	c2 := make(chan int)
	chm := map[chan int]string{c1: "c1", c2: "c2"}
	fmt.Println(len(chm), chm[c1], chm[c2])

	pm := map[*Pt]int{}
	p1, p2 := &Pt{1, 1}, &Pt{1, 1}
	pm[p1] = 1
	pm[p2] = 2
	fmt.Println(len(pm), pm[p1], pm[p2], pm[&Pt{1, 1}])

	// Interface key of a pointer-free struct type with an interface field.
	type W struct{ V any }
	wm := map[W]int{{1}: 1, {"1"}: 2, {nil}: 3}
	fmt.Println(wm[W{1}], wm[W{"1"}], wm[W{}], wm[W{1.0}])

	// Complex keys.
	xm := map[complex128]int{1 + 2i: 1, 2 + 1i: 2}
	xm[complex(1, 2)] += 10
	fmt.Println(xm, len(xm))
}
