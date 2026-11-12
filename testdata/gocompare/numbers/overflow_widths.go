// Addition, subtraction, multiplication and negation wrap at the width of
// every integer type, for values near the limits, with variables in loops.
package main

import (
	"fmt"
	"math"
)

func main() {
	i8s := []int8{math.MinInt8, math.MinInt8 + 1, -1, 0, 1, math.MaxInt8 - 1, math.MaxInt8}
	for _, a := range i8s {
		for _, b := range i8s {
			fmt.Print(a+b, ",", a-b, ",", a*b, " ")
		}
		fmt.Println(-a, ^a)
	}
	u8s := []uint8{0, 1, 2, 127, 128, 254, 255}
	for _, a := range u8s {
		for _, b := range u8s {
			fmt.Print(a+b, ",", a-b, ",", a*b, " ")
		}
		fmt.Println(-a, ^a)
	}
	i16s := []int16{math.MinInt16, -300, -1, 0, 1, 300, math.MaxInt16}
	for _, a := range i16s {
		for _, b := range i16s {
			fmt.Print(a+b, ",", a-b, ",", a*b, " ")
		}
		fmt.Println(-a, ^a)
	}
	u16s := []uint16{0, 1, 255, 256, 32768, 65534, 65535}
	for _, a := range u16s {
		for _, b := range u16s {
			fmt.Print(a+b, ",", a-b, ",", a*b, " ")
		}
		fmt.Println(-a, ^a)
	}
	i32s := []int32{math.MinInt32, -70000, -1, 0, 1, 70000, math.MaxInt32}
	for _, a := range i32s {
		for _, b := range i32s {
			fmt.Print(a+b, ",", a-b, ",", a*b, " ")
		}
		fmt.Println(-a, ^a)
	}
	u32s := []uint32{0, 1, 65536, 1 << 31, math.MaxUint32 - 1, math.MaxUint32}
	for _, a := range u32s {
		for _, b := range u32s {
			fmt.Print(a+b, ",", a-b, ",", a*b, " ")
		}
		fmt.Println(-a, ^a)
	}
	i64s := []int64{math.MinInt64, -5000000000, -1, 0, 1, 5000000000, math.MaxInt64}
	for _, a := range i64s {
		for _, b := range i64s {
			fmt.Print(a+b, ",", a-b, ",", a*b, " ")
		}
		fmt.Println(-a, ^a)
	}
	u64s := []uint64{0, 1, 1 << 32, 1 << 63, math.MaxUint64 - 1, math.MaxUint64}
	for _, a := range u64s {
		for _, b := range u64s {
			fmt.Print(a+b, ",", a-b, ",", a*b, " ")
		}
		fmt.Println(-a, ^a)
	}
	// int and uint with values that fit 32 bits, wrapping at 32 bits too.
	var ui uint = 0
	ui--
	fmt.Println(uint32(ui), uint16(ui))
	var ii int = 1 << 30
	fmt.Println(int32(ii*2), int32(ii*4), ii/3)
	var p uintptr = 0
	p--
	fmt.Println(uint32(p), uint8(p))
	// Repeated multiplication: factorials wrap.
	f8, f16, f32, f64 := int8(1), int16(1), int32(1), int64(1)
	g8, g16, g32, g64 := uint8(1), uint16(1), uint32(1), uint64(1)
	for k := 1; k <= 25; k++ {
		f8 *= int8(k)
		f16 *= int16(k)
		f32 *= int32(k)
		f64 *= int64(k)
		g8 *= uint8(k)
		g16 *= uint16(k)
		g32 *= uint32(k)
		g64 *= uint64(k)
		fmt.Println(k, f8, f16, f32, f64, g8, g16, g32, g64)
	}
	// Fibonacci wraps.
	a, b := uint32(0), uint32(1)
	c, d := int16(0), int16(1)
	for k := 0; k < 60; k++ {
		a, b = b, a+b
		c, d = d, c+d
	}
	fmt.Println(a, b, c, d)
}
