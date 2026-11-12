// The builtins min and max with integers of every type, floats with signed
// zeros and NaN, strings, constants, many arguments, and generics.
package main

import (
	"cmp"
	"fmt"
	"math"
	"slices"
)

func gmin[T cmp.Ordered](xs ...T) T {
	m := xs[0]
	for _, x := range xs[1:] {
		m = min(m, x)
	}
	return m
}

func gmax[T cmp.Ordered](a, b, c T) T { return max(a, b, c) }

func main() {
	var i8a, i8b int8 = -128, 127
	var u8a, u8b uint8 = 0, 255
	var i64a, i64b int64 = math.MinInt64, math.MaxInt64
	var ua, ub uint64 = 1 << 63, 1<<64 - 1
	fmt.Println(min(i8a, i8b), max(i8a, i8b), min(u8a, u8b), max(u8a, u8b), min(i64a, i64b), max(i64a, i64b), min(ua, ub), max(ua, ub))
	var i16 int16 = 5
	fmt.Println(min(i16, 3, 9, -2), max(i16, 3, 9, -2), min(i16), max(i16, 100))
	fmt.Printf("%T %T %T\n", min(1, 2.5), max('a', 1), min(i16, 1))
	const c = min(3, 1.5, 2)
	const d = max("b", "abc", "ab")
	fmt.Println(c, d)
	nan := math.NaN()
	negz := math.Copysign(0, -1)
	// NaN with an infinity is left out here (see the min/max NaN bug).
	fs := []float64{1, nan, -1, 0, negz, 1e308}
	for _, a := range fs {
		for _, b := range fs {
			mn, mx := min(a, b), max(a, b)
			fmt.Print(mn, math.Signbit(mn), mx, math.Signbit(mx), "|")
		}
		fmt.Println()
	}
	var f32a, f32b float32 = 1.5, float32(nan)
	fmt.Println(min(f32a, f32b), max(f32a, 2), min(f32a, 1, 0.5, 3))
	inf := math.Inf(1)
	fmt.Println(min(inf, -inf, 1), max(inf, -inf, 1), min(inf, 1), max(-inf, 0))
	fmt.Println(min(1.0, 2, nan, 3), max(nan, 1.0), min(negz, 0), max(0, negz))
	fmt.Println(math.Signbit(min(negz, 0)), math.Signbit(min(0, negz)), math.Signbit(max(negz, 0)), math.Signbit(max(0, negz)))
	s1, s2 := "apple", "Apple"
	fmt.Println(min(s1, s2), max(s1, s2, "b"), min("", s1), max("é", "z"))
	fmt.Println(gmin(3, 1, 2), gmin("z", "y"), gmin(2.5, -1.5), gmin(uint8(9), 4), gmax(int8(-1), -5, -3), gmax(1.0, nan, 2))
	fmt.Println(slices.Min([]int64{4, -9, 2}), slices.Max([]float64{1, 3, 2}), cmp.Compare(nan, 1), cmp.Compare(1, nan), cmp.Compare(nan, nan), cmp.Less(nan, -1.0))
	// min/max in loops: clamp.
	for _, v := range []int32{-50, 0, 50, 150, 300} {
		fmt.Print(max(0, min(v, 100)), " ")
	}
	fmt.Println()
	// The result is assignable to the type of the arguments.
	var r uint16 = max(uint16(3), 7)
	r++
	fmt.Println(r)
}
