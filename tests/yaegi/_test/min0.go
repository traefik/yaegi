package main

import (
	"fmt"
	"math"
)

type myInt int

const c0 = min(3, 1.5, 2)

const c1 int8 = min(5, 4)

func minInt(a, b int) int { return min(a, b) }

func main() {
	a, b, c := 3, -1, 2
	fmt.Println(min(a), min(a, b, c), min(a, 10), min(-5, a))

	var u1, u2 uint8 = 200, 7
	fmt.Println(min(u1, u2), min(u1, 255))

	f := 2.5
	fmt.Println(min(f, 1), min(1, f, 0.5))

	var f32 float32 = 1.25
	fmt.Println(min(f32, 2))

	nan := math.NaN()
	fmt.Println(min(nan, 1.0), min(1.0, nan), min(f, nan, -10))

	negz := math.Copysign(0, -1)
	posz := 0.0
	fmt.Println(math.Signbit(min(posz, negz)), math.Signbit(min(negz, posz)))

	s1, s2 := "banana", "apple"
	fmt.Println(min(s1, s2), min(s1, "cherry"), min("b", "a", "c"))

	fmt.Printf("%v %T\n", c0, c0)
	fmt.Printf("%v %T\n", c1, c1)
	fmt.Printf("%v %T\n", min(1, 2), min(1, 2))
	fmt.Printf("%v %T\n", min('a', 98), min('a', 98))

	var m myInt = 4
	var m2 myInt = min(m, 2)
	fmt.Println(m2)

	var i interface{} = min(a, b)
	fmt.Println(i, minInt(7, 3))
}

// Output:
// 3 -1 3 -5
// 7 200
// 1 0.5
// 1.25
// NaN NaN NaN
// true true
// apple banana a
// 1.5 float64
// 4 int8
// 1 int
// 97 int32
// 2
// -1 3
