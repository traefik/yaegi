package main

import (
	"fmt"
	"math"
)

const c0 = max(1, 2.5)

const c1 = max("a", "b")

func maxFloat(a, b float64) float64 { return max(a, b) }

func main() {
	a, b, c := 3, -1, 2
	fmt.Println(max(a), max(a, b, c), max(a, 10), max(-5, a))

	var u1, u2 uint64 = 1 << 40, 7
	fmt.Println(max(u1, u2), max(u2, 3))

	f := 2.5
	fmt.Println(max(f, 1), max(1, f, 3.5))

	nan := math.NaN()
	fmt.Println(max(nan, 1.0), max(1.0, nan), max(f, nan, 10))

	negz := math.Copysign(0, -1)
	posz := 0.0
	fmt.Println(math.Signbit(max(posz, negz)), math.Signbit(max(negz, posz)), math.Signbit(max(negz, negz)))

	s1, s2 := "banana", "apple"
	fmt.Println(max(s1, s2), max(s2, "cherry"))

	fmt.Printf("%v %T\n", c0, c0)
	fmt.Printf("%v %T\n", c1, c1)
	fmt.Printf("%v %T\n", max(2, 1.0), max(2, 1.0))

	var x float32 = max(1, 2)
	fmt.Printf("%v %T\n", x, x)

	fmt.Println(maxFloat(1.5, -2))

	var arr [max(2, 3)]int
	fmt.Println(len(arr))
}

// Output:
// 3 3 10 3
// 1099511627776 7
// 2.5 3.5
// NaN NaN NaN
// false false true
// banana cherry
// 2.5 float64
// b string
// 2 float64
// 2 float32
// 1.5
// 3
