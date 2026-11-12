package main

// reflect.DeepEqual and the other functions of the twin reflect with `any`
// parameters (vm/twin_fallback.go) get the interpreted values boxed, as in
// AOT mode. In default mode they got native copies: DeepEqual(A{1}, B{1})
// was true (two copies of struct { X int }), and a slice that holds a NaN
// was not equal to itself (two copies).
// (From the reflect battery of the modes challenge.)
//
// reflect.DeepEqual on interpreted types: structs, maps, slices,
// interfaces, cycles, unexported fields, funcs, NaN.

import (
	"fmt"
	"math"
	"reflect"
)

type Node struct {
	V    int
	Next *Node
	tags []string
}

type Shape interface{ Area() float64 }
type Sq struct{ S float64 }

func (s Sq) Area() float64 { return s.S * s.S }

type A struct{ X int }
type B struct{ X int }

type Wrap struct {
	S Shape
	M map[string]any
}

func main() {
	a := &Node{V: 1, tags: []string{"x"}}
	a.Next = a
	b := &Node{V: 1, tags: []string{"x"}}
	b.Next = b
	fmt.Println(reflect.DeepEqual(a, b))
	b.tags = []string{"y"}
	fmt.Println(reflect.DeepEqual(a, b))

	fmt.Println(reflect.DeepEqual(A{1}, B{1}), reflect.DeepEqual(A{1}, A{1}))
	fmt.Println(reflect.DeepEqual(Wrap{Sq{2}, map[string]any{"k": []int{1}}}, Wrap{Sq{2}, map[string]any{"k": []int{1}}}))
	fmt.Println(reflect.DeepEqual(Wrap{S: Sq{2}}, Wrap{S: Sq{3}}))
	fmt.Println(reflect.DeepEqual([]Shape{Sq{1}, nil}, []Shape{Sq{1}, nil}))
	fmt.Println(reflect.DeepEqual([]int{}, []int(nil)), reflect.DeepEqual(map[A]int{{1}: 1}, map[A]int{{1}: 1}))
	var f func()
	fmt.Println(reflect.DeepEqual(f, f), reflect.DeepEqual(func() {}, func() {}))
	nan := math.NaN()
	fmt.Println(reflect.DeepEqual(nan, nan), reflect.DeepEqual([]float64{nan}, []float64{nan}))
	s := []float64{nan}
	fmt.Println(reflect.DeepEqual(s, s))
	var e1, e2 any = A{1}, A{1}
	fmt.Println(reflect.DeepEqual(e1, e2), e1 == e2, reflect.DeepEqual(&A{1}, &A{1}), &A{1} == &A{1})
	type Local struct{ M map[A][]B }
	fmt.Println(reflect.DeepEqual(Local{map[A][]B{{1}: {{2}}}}, Local{map[A][]B{{1}: {{2}}}}))

	// reflect.Swapper: slices of the program, and the panics of gc on a
	// value that is not a slice (the VM swapped the cells of a struct)
	ints := []int{3, 1, 2}
	reflect.Swapper(ints)(0, 2)
	as := []A{{1}, {2}}
	reflect.Swapper(as)(0, 1)
	sh := []Shape{Sq{1}, nil}
	reflect.Swapper(sh)(0, 1)
	fmt.Println(ints, as, sh)
	try := func(f func()) {
		defer func() { fmt.Println("recovered:", recover()) }()
		f()
	}
	try(func() { reflect.Swapper(A{1}) })
	try(func() { reflect.Swapper(nil) })
	try(func() { reflect.Swapper(&ints) })
}
