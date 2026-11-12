// complex64 and complex128 arithmetic, the builtins complex, real and imag,
// comparisons, division by zero, constants, and fmt of complex values. The
// operands of products have exact component products: gc may compute
// re = a*c - b*d with one FMA on some platforms.
package main

import (
	"fmt"
	"math"
	"math/cmplx"
)

func main() {
	a := complex(1.5, -2)
	b := 3 + 4i
	fmt.Println(a+b, a-b, a*b, a/b, -a, a == b, a != b)
	fmt.Println(real(a), imag(a), real(b*b), imag(b*b))
	var c64 complex64 = complex(float32(0.5), float32(-0.75))
	d64 := complex64(3 - 1i)
	fmt.Println(c64+d64, c64*d64, c64/d64, c64-d64, real(c64), imag(c64))
	fmt.Printf("%T %T %T %T\n", real(c64), imag(a), complex(float32(1), 2), complex(1, 2))
	fmt.Println(complex128(c64), complex64(a), real(complex128(c64)))
	// Division by zero is not a panic for complex.
	var z complex128
	r := a / z
	fmt.Println(cmplx.IsInf(r), cmplx.IsNaN(z/z), real(1/z) > 0)
	// Compound assignment.
	acc := complex(1, 1)
	for i := 0; i < 8; i++ {
		acc *= complex(1, 1)
	}
	fmt.Println(acc)
	acc = complex128(acc / 2i)
	acc += 1
	acc -= 1i
	fmt.Println(acc)
	var acc64 complex64 = 1
	for i := 0; i < 10; i++ {
		acc64 *= complex(float32(0.5), float32(0.25))
	}
	fmt.Println(acc64)
	// cmplx functions with exact or well-defined results.
	fmt.Println(cmplx.Abs(b), cmplx.Conj(b), cmplx.Sqrt(-4), cmplx.Inf(), cmplx.IsInf(cmplx.Inf()))
	fmt.Printf("%.6f %.6f %.6f\n", cmplx.Exp(complex(0, math.Pi)), cmplx.Pow(1i, 2), cmplx.Phase(-1))
	fmt.Printf("%.6f %.6f\n", cmplx.Log(1i), cmplx.Abs(complex(0, 2)))
	pr, pt := cmplx.Polar(complex(0, 2))
	fmt.Printf("%.6f %.6f %.6f\n", pr, pt, cmplx.Rect(pr, pt))
	// Constants.
	const k = 2i * 2i
	const k2 = complex(3, 0)
	var f float64 = real(k2)
	var g = k + 1
	fmt.Println(k, f, g, imag(k2))
	fmt.Printf("%T %v\n", g, g)
	// Formats.
	fmt.Printf("%v|%.2f|%8.1f|%e|%g|%+.1f|%G\n", a, a, b, b, c64, b, complex(1e21, -1e-7))
	fmt.Printf("%v %v\n", complex(math.NaN(), math.Inf(-1)), complex(math.Copysign(0, -1), 0))
	// Comparisons of complex in arrays and maps.
	m := map[complex128]string{1i: "i", 1: "one", -1: "minus one"}
	fmt.Println(m)
	fmt.Println([2]complex64{1, 2i} == [2]complex64{1, 2i})
	// Complex from variables of each float type.
	x32, y32 := float32(1.25), float32(-0.5)
	x64, y64 := 1.5e10, -2.5e10
	fmt.Println(complex(x32, y32), complex(x64, y64)*complex(x64, y64))
	// Conversion of untyped constant to complex.
	var cc complex128 = 5
	cc = cc * 2
	fmt.Println(cc, real(cc)+imag(cc))
}
