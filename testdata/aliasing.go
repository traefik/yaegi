// Value semantics and aliasing: copy and append (shared arrays, growth,
// full slice expressions), arrays copied by value, structs with arrays,
// struct and array comparison, maps with struct, array, pointer, interface,
// float and channel keys, map values that are structs (copied on read),
// pointer to array elements and fields, and init order of package
// variables.
package main

import (
	"fmt"
	"slices"
	"sort"
)

type Point struct{ X, Y int }

type Rect struct {
	Min, Max Point
	Tags     [2]string
}

type Key struct {
	Name string
	Pos  [2]int
	P    *int
	I    any
}

// Package variables are initialized in dependency order, not source
// order.
var (
	a1 = b1 + 1
	b1 = c1 * 2
	c1 = initC()
)

var initLog []string

func initC() int {
	initLog = append(initLog, "initC")
	return 3
}

func init() { initLog = append(initLog, "init1") }
func init() { initLog = append(initLog, "init2") }

func main() {
	fmt.Println(a1, b1, c1, initLog)

	// append shares the array while capacity allows.
	base := make([]int, 3, 5)
	x := append(base, 1)
	y := append(base, 2) // overwrites x[3]
	fmt.Println(x, y, len(base), cap(base))
	z := append(base[:3:3], 9) // full slice expression forces a copy
	z[0] = 100
	fmt.Println(base, z)
	grown := append(x, 5, 6, 7) // past cap: new array
	grown[0] = -1
	fmt.Println(x[0], grown[0])

	// copy with overlap and with strings.
	s := []int{1, 2, 3, 4, 5}
	n := copy(s[1:], s)
	fmt.Println(n, s)
	bs := make([]byte, 3)
	n = copy(bs, "héllo")
	fmt.Println(n, bs)
	fmt.Println(copy(s, []int{}), copy([]int(nil), s))

	// Arrays are values.
	arr := [3]int{1, 2, 3}
	arr2 := arr
	arr2[0] = 9
	pa := &arr
	pa[1] = 8
	sl := arr[:]
	sl[2] = 7
	fmt.Println(arr, arr2, arr == [3]int{1, 8, 7}, len(pa))
	for i, v := range arr { // range over an array copies it
		arr[2] = 100
		if i == 2 {
			fmt.Println("range copy:", v)
		}
	}
	for i, v := range &arr { // range over a pointer does not
		arr[2] = 200
		if i == 2 {
			fmt.Println("range ptr:", v)
		}
	}

	// Structs with arrays are copied deeply.
	r1 := Rect{Point{0, 0}, Point{1, 1}, [2]string{"a", "b"}}
	r2 := r1
	r2.Tags[0] = "z"
	r2.Max.X = 5
	fmt.Println(r1, r2, r1 == r2, r1 == Rect{Max: Point{1, 1}, Tags: [2]string{"a", "b"}})

	// Pointers to elements and fields.
	pts := []Point{{1, 2}, {3, 4}}
	pp := &pts[1]
	pp.X = 30
	px := &pts[0].Y
	*px = 20
	fmt.Println(pts)
	pts = append(pts, Point{5, 6}) // may move the array: pp keeps the old one
	pp.Y = 99
	fmt.Println(pts[1].Y == 99 || pts[1].Y == 4)

	// Map values are copied on read; map of pointers is not.
	mv := map[string]Point{"a": {1, 1}}
	p := mv["a"]
	p.X = 9
	fmt.Println(mv["a"], p)
	mp := map[string]*Point{"a": {1, 1}}
	mp["a"].X = 9
	fmt.Println(*mp["a"])
	ms := map[string][]int{"a": {1}}
	ms["a"][0] = 5
	ms["a"] = append(ms["a"], 6)
	fmt.Println(ms)

	// Composite map keys.
	one, two := 1, 1
	km := map[Key]string{}
	km[Key{"a", [2]int{1, 2}, &one, 1}] = "first"
	km[Key{"a", [2]int{1, 2}, &one, 1}] = "same key"
	km[Key{"a", [2]int{1, 2}, &two, 1}] = "other pointer"
	km[Key{"a", [2]int{1, 2}, &one, int64(1)}] = "other dynamic type"
	km[Key{"a", [2]int{1, 2}, &one, Point{1, 2}}] = "struct in iface"
	var vals []string
	for _, v := range km {
		vals = append(vals, v)
	}
	sort.Strings(vals)
	fmt.Println(len(km), vals, km[Key{"a", [2]int{1, 2}, &one, Point{1, 2}}])

	am := map[[2]Point]int{{{1, 2}, {3, 4}}: 1}
	am[[2]Point{{1, 2}, {3, 4}}]++
	fmt.Println(am)

	im := map[any]string{1: "int", "1": "string", 1.0: "float", Point{1, 1}: "point", [1]int{1}: "array", true: "bool", nil: "nil"}
	fmt.Println(len(im), im[1], im[1.0], im[Point{1, 1}], im[[1]int{1}], im[nil], im[int8(1)] == "")

	ch1, ch2 := make(chan int), make(chan int)
	cm := map[chan int]int{ch1: 1, ch2: 2}
	fmt.Println(cm[ch1], cm[ch2], len(cm))

	type E struct{}
	em := map[E]int{{}: 1}
	em[E{}]++
	fmt.Println(em[E{}])

	// Deleting during range is allowed.
	dm := map[int]bool{}
	for i := range 100 {
		dm[i] = true
	}
	for k := range dm {
		if k%2 == 0 {
			delete(dm, k)
		}
	}
	fmt.Println(len(dm))

	// Struct comparison with interfaces and nested arrays.
	type S struct {
		A [2]any
		B string
	}
	fmt.Println(S{[2]any{1, "x"}, "b"} == S{[2]any{1, "x"}, "b"}, S{[2]any{1, nil}, ""} == S{})
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		fmt.Println(S{[2]any{[]int{}, nil}, ""} == S{})
	}()

	// Slices of slices share.
	grid := make([][]int, 2)
	row := []int{0, 0}
	grid[0], grid[1] = row, row
	grid[0][1] = 5
	fmt.Println(grid)
	cl := slices.Clone(grid)
	cl[0] = []int{7}
	fmt.Println(grid, cl)

	// Nil and empty slices.
	var nilS []int
	empty := []int{}
	fmt.Println(nilS == nil, empty == nil, len(append(nilS, empty...)), append(nilS, empty...) == nil)
	fmt.Println(slices.Equal(nilS, empty))

	// Swap and multiple assignment order.
	i, j := 0, 1
	vs := []int{10, 20}
	i, vs[i] = 1, 99 // index evaluated before assignment
	fmt.Println(i, j, vs)
	vs[0], vs[1] = vs[1], vs[0]
	fmt.Println(vs)
}
