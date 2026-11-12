// fmt verbs on composite values: slices, arrays, maps (printed sorted,
// with mixed and NaN keys), structs (named, anonymous, embedded,
// unexported fields), pointers to structs at the top level (&{...}),
// nil pointers, interfaces, funcs and chans only as nil, with
// %v %+v %#v %T %x %q %s %d.
package main

import (
	"fmt"
	"math"
)

type Point struct {
	X, Y int
}

type Inner struct {
	Name string
	tags []string
}

type Outer struct {
	Inner
	P     Point
	Ptr   *Point
	M     map[string]int
	Any   any
	Err   error
	F     func()
	Ch    chan int
	Arr   [2]bool
	bytes []byte
}

type Empty struct{}

type Celsius float64

func main() {
	vals := []any{
		[]int{1, 2, 3},
		[]int(nil),
		[]int{},
		[2]string{"a", "b c"},
		[]string{"x", "", "é"},
		[][]int{{1}, {2, 3}, nil},
		map[string]int{"b": 2, "a": 1, "c": 3},
		map[int]string{3: "c", -1: "z", 10: "ten"},
		map[string]int(nil),
		map[bool][]int{true: {1}, false: nil},
		map[float64]string{math.NaN(): "nan", 1: "one", math.Inf(-1): "-inf", -0.5: "neg"},
		map[any]int{"s": 1, "b": 2, "a": 5},
		map[[2]int]string{{2, 1}: "x", {1, 9}: "y", {1, 2}: "z"},
		map[Point]bool{{2, 1}: true, {1, 2}: false},
		Point{1, -2},
		&Point{3, 4},
		(*Point)(nil),
		struct{}{},
		Empty{},
		&Empty{},
		struct {
			A int
			b string
		}{1, "x"},
		Outer{Inner: Inner{"in", []string{"t1"}}, P: Point{5, 6}, M: map[string]int{"k": 1}, Any: 3.5, Arr: [2]bool{true, false}, bytes: []byte("hi")},
		&Outer{Any: Point{7, 8}},
		[]Point{{1, 2}, {3, 4}},
		[]any{1, "a", nil, 2.5, []int{1}},
		[]error{nil},
		Celsius(36.6),
		[]Celsius{1, 2.5},
		[3]*Point{},
		map[string]*Point{"nil": nil},
		[]byte(nil),
		[]uint8{1, 2},
		[]int8{-1, 2},
		[]rune("hé"),
		[]float64{1, 2.5, math.Inf(1)},
		nil,
	}
	for _, v := range vals {
		fmt.Printf("%v | %+v | %#v | %T\n", v, v, v, v)
	}
	ints := []int{10, 255, -3}
	fmt.Printf("%x %X %#x %d %5d %-3d| %08b %q %c %U\n", ints, ints, ints, ints, ints, ints, ints, ints, ints, ints)
	strs := []string{"a", "b\n", "日"}
	fmt.Printf("%s %q %x %X % x %5s %-3s| %#q\n", strs, strs, strs, strs, strs, strs, strs, strs)
	fmt.Printf("%x %X % x\n", [][]byte{[]byte("ab"), []byte("c")}, []string{"ab", "c"}, [2][]byte{[]byte("z")})
	m := map[string][]string{"x": {"1", "2"}, "a": nil}
	fmt.Printf("%v %q %#v %d\n", m, m, m, m)
	pt := &Point{1, 2}
	fmt.Printf("%v %+v %#v %s %d %x %T\n", pt, pt, pt, pt, pt, pt, pt)
	pp := &pt
	_ = pp
	fmt.Printf("%v %+v %d\n", *pp, **pp, **pp)
	var ip *int
	var ep error
	var ia any
	var fn func()
	var ch chan int
	var sl []string
	var mp map[int]int
	fmt.Printf("%v %v %v %v %v %v %v\n", ip, ep, ia, fn == nil, ch, sl, mp)
	fmt.Printf("%#v %#v %#v %#v %#v %#v\n", ip, ep, ia, ch, sl, mp)
	fmt.Printf("%d %s %x %q %t\n", ip, ep, ia, sl, mp)
	fmt.Printf("%T %T %T %T %T %T %T\n", ip, ep, ia, fn, ch, sl, mp)
	fmt.Printf("%T %T %T %T\n", struct{ A int }{}, [0]int{}, map[string][]*Point{}, func(int, ...string) (bool, error) { return false, nil })
	fmt.Printf("%T %T %T\n", make(<-chan int), make(chan<- []byte), any(Celsius(0)))
	fmt.Printf("%v %d\n", [3]Point{{1, 2}}, [2][2]int{{1, 2}, {3, 4}})
	fmt.Printf("%6.2f|%-8v|%8v|\n", []float64{1, 2.345}, []int{1, 2}, Point{1, 2})
	fmt.Println(Outer{}.Arr, Point{}, &Point{}, []*Point{nil}, struct{ P *Point }{})
	fmt.Print(Point{1, 2}, &Point{3, 4}, "\n")
}
