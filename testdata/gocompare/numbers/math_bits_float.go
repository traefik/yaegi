// The bit-level float functions of math: Float64bits, Float64frombits,
// Float32bits, Float32frombits, Nextafter, Nextafter32, Frexp, Ldexp, Ilogb,
// Logb, FMA, Inf, NaN, IsNaN, IsInf, and the math limit constants.
package main

import (
	"fmt"
	"math"
)

func main() {
	negz := math.Copysign(0, -1)
	for _, f := range []float64{0, negz, 1, -1, 0.1, 2, 0.5, math.Pi, math.MaxFloat64, math.SmallestNonzeroFloat64,
		math.Inf(1), math.Inf(-1), 1e-310, 1 << 52, 123.456} {
		b := math.Float64bits(f)
		fmt.Printf("%v %#016x %v\n", f, b, math.Float64frombits(b) == f)
		fr, ex := math.Frexp(f)
		fmt.Println("  frexp", fr, ex, math.Ldexp(fr, ex), math.Ilogb(f), math.Logb(f))
		fmt.Println("  next", math.Nextafter(f, math.Inf(1)), math.Nextafter(f, math.Inf(-1)), math.Nextafter(f, f))
	}
	for _, f := range []float32{0, 1, -1, 0.1, 3.4028235e38, 1.4e-45, 16777216, -2.5} {
		b := math.Float32bits(f)
		fmt.Printf("%v %#08x %v %v %v\n", f, b, math.Float32frombits(b), math.Nextafter32(f, 10), math.Nextafter32(f, -10))
	}
	for _, b := range []uint64{0x3FF0000000000000, 0x8000000000000000, 0x0000000000000001, 0x7FEFFFFFFFFFFFFF, 0x4009_21FB_5444_2D18, 0xC000000000000000} {
		fmt.Println(math.Float64frombits(b))
	}
	for _, b := range []uint32{0x3F800000, 0x80000000, 1, 0x7F7FFFFF, 0x40490FDB, 0xFF800000} {
		fmt.Println(math.Float32frombits(b))
	}
	// FMA: one rounding only.
	x, y, z := 1.0+math.Pow(2, -30), 1.0-math.Pow(2, -30), -1.0
	fmt.Println(math.FMA(x, y, z), float64(x*y)+z)
	fmt.Println(math.FMA(2, 3, 4), math.FMA(0.1, 10, -1), math.FMA(math.Inf(1), 0, 1), math.FMA(1e308, 10, -1e308))
	fmt.Println(math.FMA(negz, 1, negz), math.Signbit(math.FMA(negz, 1, negz)))
	// Ldexp edge cases.
	fmt.Println(math.Ldexp(1, 1023), math.Ldexp(1, 1024), math.Ldexp(1, -1074), math.Ldexp(1, -1075), math.Ldexp(-3, 4))
	fmt.Println(math.Ilogb(0), math.Ilogb(math.NaN()), math.Ilogb(math.Inf(1)), math.Logb(0), math.Logb(math.Inf(-1)))
	// Inf and NaN.
	fmt.Println(math.Inf(0), math.Inf(-5), math.Inf(3), math.IsNaN(math.NaN()), math.IsNaN(1), math.IsInf(math.Inf(-1), 1))
	// Limits.
	fmt.Println(math.MaxInt8, math.MinInt8, math.MaxInt16, math.MinInt16, math.MaxInt32, math.MinInt32)
	fmt.Println(int64(math.MaxInt64), int64(math.MinInt64), uint64(math.MaxUint64), math.MaxUint8, math.MaxUint16, uint32(math.MaxUint32))
	fmt.Println(math.MaxFloat32, math.SmallestNonzeroFloat32, math.MaxFloat64, math.SmallestNonzeroFloat64)
	fmt.Println(math.E, math.Pi, math.Phi, math.Sqrt2, math.SqrtE, math.SqrtPi, math.SqrtPhi, math.Ln2, math.Log2E, math.Ln10, math.Log10E)
	// Walk float32 neighbors of 1.
	f := float32(1)
	for i := 0; i < 5; i++ {
		f = math.Nextafter32(f, 2)
		fmt.Print(f, " ", math.Float32bits(f), " ")
	}
	fmt.Println()
	g := 1.0
	for i := 0; i < 3; i++ {
		g = math.Nextafter(g, 0)
		fmt.Print(g, " ")
	}
	fmt.Println()
}
