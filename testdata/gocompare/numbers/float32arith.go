// float32 arithmetic rounds each result to float32, as in compiled Go:
// sums, products, divisions, accumulations and comparisons with float64.
// Each product or quotient that feeds an addition is converted, so that gc
// may not fuse them into one FMA (the result would depend on the platform).
package main

import (
	"fmt"
	"math"
)

func main() {
	var a, b float32 = 0.1, 0.2
	fmt.Println(a+b, a*b, a/b, a-b, a+b == 0.3, float64(a+b))
	var sum float32
	for i := 0; i < 1000; i++ {
		sum += 0.01
	}
	fmt.Println(sum, float64(sum))
	var big float32 = 16777216
	fmt.Println(big+1, big+2, big+1 == big, big*big, big*big*big*big*big*big*big*big)
	var x float32 = 1
	for i := 0; i < 30; i++ {
		x /= 3
	}
	fmt.Println(x, x*1e10, float64(x))
	var h float32 = 1e-38
	fmt.Println(h/10, h/1e7, h/1e8, h/1e9)
	var m float32 = math.MaxFloat32
	fmt.Println(m, m*2, -m*2, m+float32(m/1e8), m+float32(m/1e7))
	vals := []float32{1, 3, 7, 0.1, 1e-5, 123456.789, -2.5}
	for _, p := range vals {
		for _, q := range vals {
			fmt.Print(p*q, " ", p/q, " ", p+q, " ", p-q, "|")
		}
		fmt.Println()
	}
	// Mixed: float32 promoted only by explicit conversion.
	var f32 float32 = 1.1
	var f64 float64 = 1.1
	fmt.Println(float64(f32) == f64, f32 == float32(f64), float64(f32)-f64)
	// Accumulate a harmonic series.
	var hs32 float32
	var hs64 float64
	for i := 1; i <= 2000; i++ {
		hs32 += float32(1 / float32(i))
		hs64 += float64(1 / float64(i))
	}
	fmt.Println(hs32, hs64, float32(hs64))
	// An explicit conversion forces the rounding of the product (Go may
	// fuse x*y+z into one FMA, so that form is not printed).
	var p, q, r float32 = 1.0000001, 1.0000001, -1.0000002
	fmt.Println(float32(p*q)+r, float64(float32(p*q)))
	// Comparisons.
	fmt.Println(a < b, a > b, a <= a, a != b, -a < 0)
	// Increment and compound on float32.
	z := float32(0.5)
	z++
	z = float32(z * 3)
	z -= 0.25
	z /= 7
	fmt.Println(z)
	fmt.Printf("%v %g %e %.10f %b\n", z, z, z, z, z)
	fmt.Println(math.Float32bits(z), math.Float32bits(a+b))
	// Conversion of float32 to string.
	fmt.Println(fmt.Sprint(float32(1)/3), fmt.Sprint(float64(float32(1)/3)))
}
