// Conversions between integers and floats with variables, for values in
// range: truncation toward zero, rounding of large integers to float32 and
// float64, and float64 to float32 rounding.
package main

import (
	"fmt"
	"math"
)

func main() {
	fs := []float64{0, -0.0, 0.4, 0.5, 0.6, 1.5, -1.5, 2.5, -2.5, 99.99, -99.99, 127.9, -128.9, 255.5,
		32767.75, -32768.75, 65535.2, 2147483647.9, -2147483648.9, 4294967295.5, 1e15 + 0.5, -1e18}
	for _, f := range fs {
		s := fmt.Sprint(int64(f))
		if f > -129 && f < 128 {
			s += fmt.Sprint(" i8=", int8(f))
		}
		if f >= 0 && f < 256 {
			s += fmt.Sprint(" u8=", uint8(f))
		}
		if f > -32769 && f < 32768 {
			s += fmt.Sprint(" i16=", int16(f))
		}
		if f >= 0 && f < 65536 {
			s += fmt.Sprint(" u16=", uint16(f))
		}
		if f > -2147483649 && f < 2147483648 {
			s += fmt.Sprint(" i32=", int32(f))
		}
		if f >= 0 && f < 4294967296 {
			s += fmt.Sprint(" u32=", uint32(f))
		}
		if f >= 0 && f < 1.8e19 {
			s += fmt.Sprint(" u64=", uint64(f))
		}
		fmt.Println(f, s, float32(f))
	}
	// Large unsigned values to float and back.
	for _, u := range []uint64{1<<63 - 1, 1 << 63, 1<<63 + 1, 1<<64 - 1, 1<<53 + 1, 1<<24 + 1, 1<<64 - 2048, 1<<63 + 1025} {
		f := float64(u)
		g := float32(u)
		fmt.Println(u, f, g)
		if f < 1.8e19 {
			fmt.Println("  back", uint64(f))
		}
		if g < 1.8e19 {
			fmt.Println("  back32", uint64(g))
		}
	}
	for _, i := range []int64{-1 << 63, -1<<53 - 1, 1<<53 + 1, 1<<62 + 1, 1<<63 - 1, -16777217, 16777217, 123456789} {
		fmt.Println(i, float64(i), float32(i), float64(float32(i)))
	}
	for _, i := range []int32{math.MaxInt32, math.MinInt32, 16777217, -16777217, 33554435} {
		fmt.Println(i, float32(i), float64(i))
	}
	for _, i := range []uint32{math.MaxUint32, 16777217, 1 << 31} {
		fmt.Println(i, float32(i), float64(i))
	}
	for _, i := range []int8{-128, 127} {
		fmt.Println(float32(i), float64(i))
	}
	for _, i := range []uint16{0, 65535} {
		fmt.Println(float32(i), float64(i))
	}
	// float64 to float32 rounding, overflow to Inf and underflow to 0.
	for _, f := range []float64{0.1, 1.0 / 3, 1e38, 3.4028235e38, 3.4028236e38, 1e39, -1e39, 1e-45, 1e-46,
		1.401298464324817e-45, 7e-46, 0.7e-45, 16777217, 16777219, math.Pi, math.MaxFloat64, math.SmallestNonzeroFloat64} {
		g := float32(f)
		fmt.Println(f, g, float64(g), g == float32(f))
	}
	// Float to int through a named type.
	type Celsius float64
	type Count int16
	c := Celsius(-40.75)
	fmt.Println(Count(c), int(c), uint8(c+241))
	// int conversions of uint64 near float precision.
	var u uint64 = 9007199254740993
	fmt.Println(float64(u), uint64(float64(u)), float64(u-1) == float64(u))
}
