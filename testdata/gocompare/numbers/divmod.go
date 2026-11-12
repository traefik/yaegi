// Integer division and modulo truncate toward zero for every width and sign,
// MinInt/-1 wraps with variables, and division by a zero variable panics
// with a runtime error that is recovered and printed.
package main

import (
	"fmt"
	"math"
)

func try(name string, f func() string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(name, "panic:", r)
			if e, ok := r.(error); ok {
				fmt.Println(name, "error:", e.Error())
			}
		}
	}()
	fmt.Println(name, f())
}

func main() {
	ns := []int64{-17, -16, -1, 0, 1, 16, 17, math.MinInt64, math.MaxInt64}
	ds := []int64{-5, -4, -1, 1, 4, 5, math.MaxInt64, math.MinInt64}
	for _, n := range ns {
		for _, d := range ds {
			fmt.Print(n/d, " ", n%d, "|")
		}
		fmt.Println()
	}
	us := []uint64{0, 1, 16, 17, 1 << 63, math.MaxUint64}
	ud := []uint64{1, 3, 4, 1 << 32, math.MaxUint64}
	for _, n := range us {
		for _, d := range ud {
			fmt.Print(n/d, " ", n%d, "|")
		}
		fmt.Println()
	}
	var m1_8 int8 = -1
	var m1_16 int16 = -1
	var m1_32 int32 = -1
	var m1_64 int64 = -1
	var min8 int8 = math.MinInt8
	var min16 int16 = math.MinInt16
	var min32 int32 = math.MinInt32
	var min64 int64 = math.MinInt64
	fmt.Println(min8/m1_8, min8%m1_8, min16/m1_16, min16%m1_16)
	fmt.Println(min32/m1_32, min32%m1_32, min64/m1_64, min64%m1_64)
	var mi int = math.MinInt32
	var m1 int = -1
	fmt.Println(int32(mi/m1), mi%m1)
	x8 := min8
	x8 /= m1_8
	y64 := min64
	y64 %= m1_64
	fmt.Println(x8, y64)
	for _, n := range []int8{-128, -127, -7, 7, 127} {
		for _, d := range []int8{-128, -3, -1, 1, 3, 127} {
			fmt.Print(n/d, ",", n%d, " ")
		}
		fmt.Println()
	}
	for _, n := range []uint8{0, 7, 128, 255} {
		for _, d := range []uint8{1, 2, 7, 128, 255} {
			fmt.Print(n/d, ",", n%d, " ")
		}
		fmt.Println()
	}
	for _, n := range []int16{-32768, -1000, 999, 32767} {
		for _, d := range []int16{-7, 7, 100, -32768} {
			fmt.Print(n/d, ",", n%d, " ")
		}
		fmt.Println()
	}
	for _, n := range []uint32{0, 99, 1 << 31, math.MaxUint32} {
		for _, d := range []uint32{1, 10, 1 << 16, math.MaxUint32} {
			fmt.Print(n/d, ",", n%d, " ")
		}
		fmt.Println()
	}
	// Division by a constant power of two on negative values (shift tricks).
	for _, n := range []int32{-9, -8, -7, -1, 0, 1, 7, 8, 9} {
		fmt.Print(n/2, n/4, n/8, n%2, n%4, n%8, " ")
	}
	fmt.Println()
	for _, n := range []int64{-1000001, -3, 3, 1000001} {
		fmt.Print(n/3, n/7, n/10, n%3, n%7, n%10, " ")
	}
	fmt.Println()
	var z8 int8
	var z16 uint16
	var z32 int32
	var z64 uint64
	var zi int
	var zu uint
	try("int8/0", func() string { return fmt.Sprint(int8(5) / z8) })
	try("int8%0", func() string { return fmt.Sprint(int8(5) % z8) })
	try("uint16/0", func() string { return fmt.Sprint(uint16(5) / z16) })
	try("int32%0", func() string { return fmt.Sprint(min32 % z32) })
	try("uint64/0", func() string { return fmt.Sprint(uint64(1) / z64) })
	try("int/0", func() string { return fmt.Sprint(7 / zi) })
	try("uint%0", func() string { return fmt.Sprint(uint(7) % zu) })
	try("int/=0", func() string { v := 10; v /= zi; return fmt.Sprint(v) })
	try("int%=0", func() string { v := 10; v %= zi; return fmt.Sprint(v) })
	try("ok", func() string { return fmt.Sprint(10 / (zi + 3)) })
	try("float/0", func() string { f := 1.0; var z float64; return fmt.Sprint(f/z, -f/z) })
}
