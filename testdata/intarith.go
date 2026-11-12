// Integer and float arithmetic at the edges of each type: overflow must wrap
// to the width of the type, float32 results must be rounded to float32, and
// conversions must give the same results as compiled Go.
package main

import (
	"fmt"
	"math"
)

//go:noinline
func div8(x, y int8) (int8, int8)     { return x / y, x % y }
func div16(x, y int16) (int16, int16) { return x / y, x % y }
func div32(x, y int32) (int32, int32) { return x / y, x % y }
func div64(x, y int64) (int64, int64) { return x / y, x % y }
func divu8(x, y uint8) (uint8, uint8) { return x / y, x % y }

func ops8(x, y int8) []int8 {
	return []int8{x + y, x - y, x * y, -x, ^x, x << 1, x >> 1, x & y, x | y, x ^ y, x &^ y}
}

func opsu16(x, y uint16) []uint16 {
	return []uint16{x + y, x - y, x * y, -x, ^x, x << 3, x >> 3, x &^ y}
}

func opsu32(x, y uint32) []uint32 {
	return []uint32{x + y, x - y, x * y, -x, ^x, x << 5, x >> 5}
}

func shifts(x int32, u uint8, s int) (int32, int32, uint8, uint8) {
	return x << s, x >> s, u << s, u >> s
}

func f32(x, y float32) []float32 {
	return []float32{x + y, x - y, x * y, x / y, -x}
}

func shiftPanic(x, s int) (r int, err any) {
	defer func() { err = recover() }()
	return x << s, nil
}

func main() {
	fmt.Println(div8(-128, -1))
	fmt.Println(div16(-32768, -1))
	fmt.Println(div32(-2147483648, -1))
	fmt.Println(div64(math.MinInt64, -1))
	fmt.Println(divu8(255, 7))
	// Boxing in an interface can hide a wrong width: look at the value
	// through a wider type and a comparison too.
	q8, _ := div8(-128, -1)
	q16, _ := div16(-32768, -1)
	q32, _ := div32(-2147483648, -1)
	fmt.Println(int64(q8), int64(q16), int64(q32), q8 == -128, q32 < 0)
	for _, v := range ops8(-128, -1) {
		fmt.Print(int64(v), " ")
	}
	fmt.Println()
	fmt.Println(ops8(-128, -1), ops8(127, 2), ops8(100, 100))
	fmt.Println(opsu16(0, 1), opsu16(65535, 65535))
	fmt.Println(opsu32(0, 1), opsu32(4294967295, 4294967295))
	fmt.Println(shifts(-7, 200, 3))
	fmt.Println(shifts(-7, 200, 40))
	fmt.Println(shifts(1<<30, 255, 70))
	fmt.Println(shiftPanic(1, 2))
	fmt.Println(shiftPanic(1, -2))

	// float32 rounding after each operation.
	x30 := 1 << 30
	d := float32(x30)
	d += 1
	d -= float32(x30)
	fmt.Println(d, f32(1<<24, 1), f32(0.1, 3))
	var big float64 = 1e300
	fmt.Println(float32(big), float32(0.1) == 0.1, float64(float32(0.1)))

	// Constants of integer type written as float or complex literals.
	one := 1
	fmt.Println(one<<1., one<<(1+0i), one<<(1.<<one))

	// Integer to float, with unsigned values above MaxInt64.
	u := uint64(1<<63 + 1025)
	var i64 int64 = 1<<62 + 1
	fmt.Println(float64(u), float32(u), float32(i64), float64(uint32(4294967295)))

	// Float to integer, in range.
	for _, f := range []float64{-1.5, 0, 2.9, 255.9, 65535.5, -32768.9, 4.2e9, 1e18, 1.8e19} {
		fmt.Println(int8(f), uint8(f), int16(f), uint16(f), int32(f), int64(f))
		if f >= 0 {
			fmt.Println(uint32(f), uint64(f))
		}
	}
	var f3 float32 = 16777217
	f4 := float32(-3.7)
	fmt.Println(int32(f3), uint64(f3), int64(f4))

	// string(int) checks the whole value.
	big64 := uint64(0x10001f4a9)
	neg := -1
	fmt.Printf("%q %q %q\n", string(rune(big64)), string(neg), string(big64))

	// []rune conversions.
	rs := []rune("aሴc")
	rs = append(rs, 0x110000)
	fmt.Println(len(rs), rs, string(rs))
}
