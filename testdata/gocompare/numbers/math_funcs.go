// math functions with exact or well-defined results: Sqrt, Cbrt, Pow,
// Exp2, Log2, Log10, Hypot, Gamma on integers, Erf at 0, trig at 0, and the
// special cases of Pow, Log and Sqrt. Inexact ones are printed with %.10g.
package main

import (
	"fmt"
	"math"
)

func main() {
	nan := math.NaN()
	inf := math.Inf(1)
	for _, v := range []float64{0, 1, 2, 4, 9, 16, 1e10, 0.25, 2.25, -1, inf, nan} {
		fmt.Println(v, math.Sqrt(v), math.Cbrt(v*v*v), math.Log2(v), math.Log10(v))
	}
	for _, v := range []float64{-1024, -2, -1, 0, 0.5, 1, 10, 52, 1023, 1024} {
		fmt.Println(v, math.Exp2(v), math.Pow(2, v), math.Pow(10, math.Trunc(v/100)))
	}
	pows := [][2]float64{{0, 0}, {0, -1}, {math.Copysign(0, -1), -1}, {-1, inf}, {-1, -inf}, {2, inf}, {0.5, inf},
		{-8, 1.0 / 3}, {-2, 3}, {-2, -3}, {inf, -1}, {-inf, 3}, {-inf, 2}, {nan, 0}, {1, nan}, {3, 4}, {1.5, 2}}
	for _, p := range pows {
		fmt.Println(p[0], p[1], math.Pow(p[0], p[1]))
	}
	fmt.Println(math.Log(1), math.Log(0), math.Log(-1), math.Log(inf), math.Log1p(0), math.Expm1(0), math.Exp(0))
	fmt.Println(math.Sin(0), math.Cos(0), math.Tan(0), math.Asin(0), math.Acos(1), math.Atan(0), math.Atan2(0, 1), math.Atan2(0, -1))
	fmt.Println(math.Sinh(0), math.Cosh(0), math.Tanh(inf), math.Tanh(-inf), math.Erf(0), math.Erfc(0), math.Erf(inf))
	for n := 1.0; n <= 10; n++ {
		fmt.Print(math.Gamma(n), " ")
	}
	fmt.Println()
	lg, sign := math.Lgamma(-0.5)
	fmt.Printf("%.10g %d\n", lg, sign)
	fmt.Println(math.Hypot(3, 4), math.Hypot(inf, nan), math.Hypot(5, 12), math.Hypot(-8, 15))
	for _, v := range []float64{0.1, 0.5, 1.7, 3, 10, 100} {
		fmt.Printf("%.10g %.10g %.10g %.10g %.10g %.10g\n", math.Exp(v), math.Log(v), math.Sin(v), math.Cos(v), math.Atan(v), math.Sqrt(v))
	}
	fmt.Printf("%.10g %.10g %.10g\n", math.J0(1), math.Y0(1), math.Erfinv(0.5))
	s, c := math.Sincos(0)
	fmt.Println(s, c)
	fmt.Println(math.IsInf(math.Exp(1000), 1), math.Exp(-1000), math.Pow10(3), math.Pow10(-2), math.Pow10(400), math.Pow10(-400))
}
