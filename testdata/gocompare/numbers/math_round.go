// math rounding and remainder functions on a table of values with halves,
// negatives, signed zeros, infinities and NaN: Trunc, Floor, Ceil, Round,
// RoundToEven, Mod, Remainder, Modf, Abs, Copysign, Signbit, Dim, Max, Min.
package main

import (
	"fmt"
	"math"
)

func main() {
	nan := math.NaN()
	inf := math.Inf(1)
	negz := math.Copysign(0, -1)
	vals := []float64{0, negz, 0.49999999999999994, 0.5, 1.5, 2.5, -0.5, -1.5, -2.5, 3.7, -3.7,
		1e16 + 1, -4503599627370497, 4503599627370496.5, 0.1, -0.1, inf, -inf, nan, 123456.5, -7.25}
	for _, v := range vals {
		fmt.Println(v, math.Trunc(v), math.Floor(v), math.Ceil(v), math.Round(v), math.RoundToEven(v), math.Abs(v), math.Signbit(v))
		ip, fr := math.Modf(v)
		fmt.Println("  modf", ip, fr, math.Signbit(ip), math.Signbit(fr))
	}
	pairs := [][2]float64{{7, 3}, {-7, 3}, {7, -3}, {-7, -3}, {5.5, 2}, {-5.5, 2}, {6, 4}, {10, 4}, {14, 4},
		{1, 0}, {inf, 2}, {2, inf}, {nan, 1}, {1, nan}, {0, 5}, {negz, 5}, {1e300, 3}, {0.3, 0.1}, {-1, inf}}
	for _, p := range pairs {
		x, y := p[0], p[1]
		fmt.Println(x, y, math.Mod(x, y), math.Remainder(x, y), math.Dim(x, y), math.Max(x, y), math.Min(x, y),
			math.Copysign(x, y), math.Hypot(x, y))
	}
	fmt.Println(math.Max(0, negz), math.Min(0, negz), math.Signbit(math.Min(0, negz)), math.Signbit(math.Max(negz, 0)))
	fmt.Println(math.Max(inf, nan), math.Min(-inf, nan), math.Max(nan, 1))
	fmt.Println(math.Copysign(3, negz), math.Copysign(nan, -1) != 0, math.Signbit(math.Copysign(nan, -1)))
	fmt.Println(math.Abs(negz), math.Signbit(math.Abs(negz)), math.Abs(-inf))
	// Rounding to a number of decimals.
	for _, v := range []float64{1.005, 2.675, 1.45, -1.45, 12345.6789} {
		fmt.Println(math.Round(v*100)/100, math.Floor(v*10)/10, math.Ceil(v*10)/10)
	}
	// float32 values through math.
	for _, f := range []float32{0.5, 1.5, -2.5, 3.3} {
		fmt.Println(float32(math.Round(float64(f))), float32(math.Floor(float64(f))), float32(math.Trunc(float64(f))))
	}
}
