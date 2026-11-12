// Conversions between all integer types with variables: truncation, sign
// extension and zero extension.
package main

import "fmt"

func main() {
	vals := []int64{0, 1, -1, 127, 128, -128, -129, 255, 256, 32767, 32768, -32769, 65535, 65536,
		2147483647, 2147483648, -2147483649, 4294967295, 4294967296, 1<<62 + 12345, -1 << 63, 1<<63 - 1,
		0x123456789ABCDEF, -0x123456789ABCDEF}
	for _, v := range vals {
		fmt.Println(v, int8(v), int16(v), int32(v), uint8(v), uint16(v), uint32(v), uint64(v))
		u := uint64(v)
		fmt.Println(" ", int8(u), int16(u), int32(u), int64(u), uint8(u), uint16(u), uint32(u))
		i32 := int32(v)
		fmt.Println(" ", int64(i32), uint64(i32), uint32(i32), int16(i32), uint64(uint32(i32)))
		i8 := int8(v)
		fmt.Println(" ", int64(i8), uint64(i8), uint16(i8), int32(uint8(i8)), uint64(uint8(i8)))
		u16 := uint16(v)
		fmt.Println(" ", int64(u16), int16(u16), int8(u16), int32(int16(u16)))
	}
	// int, uint and uintptr with values that fit 32 bits.
	for _, v := range []int32{0, -1, 1 << 30, -1 << 31} {
		i := int(v)
		u := uint(v)
		p := uintptr(v)
		fmt.Println(i, uint32(u), uint32(p), int64(i), int8(i), uint8(u), uint16(p))
	}
	// Chains of conversions.
	var b byte = 200
	var r rune = -2
	fmt.Println(int8(b), rune(b), byte(r), uint32(r), int64(uint32(r)), int16(int8(b)))
	type MyInt int16
	type MyU uint8
	var mi MyInt = -300
	fmt.Println(MyU(mi), int64(MyU(mi)), MyInt(MyU(mi)), int32(mi))
	// Conversion of a negative to unsigned and back.
	for _, v := range []int16{-1, -2, -32768, 1} {
		u := uint16(v)
		w := uint64(u)
		fmt.Println(u, w, int16(w), int64(int16(u)), uint64(v))
	}
	// Strings from integer conversion through rune.
	for _, v := range []int32{65, 0x263A, 0x10FFFF, -1, 0xD800, 0x110000} {
		fmt.Printf("%q ", string(rune(v)))
	}
	fmt.Println()
}
