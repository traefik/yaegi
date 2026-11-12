// Integer overflow wraps to the width of each type, for each operator, with
// signed and unsigned operands, as in compiled Go.
package main

import "fmt"

func main() {
	var i8 int8 = 127
	var u8 uint8 = 255
	var i16 int16 = -32768
	var u16 uint16 = 0
	var i32 int32 = 1<<31 - 1
	var u32 uint32 = 1<<32 - 1
	var i64 int64 = -1 << 63
	var u64 uint64 = 1<<64 - 1
	fmt.Println(i8+1, u8+1, i16-1, u16-1, i32+1, u32+1, i64-1, u64+1)
	fmt.Println(i8*2, u8*2, i16*-1, i32*i32, u32*u32, i64*-1, u64*u64)
	fmt.Println(-i8, -i16, -i32, -i64, ^u8, ^u16, ^u32, ^u64)
	fmt.Println(i8<<1, u8<<4, i16<<15, i32<<31, i64<<63, u64<<63)
	fmt.Println(i8>>7, u8>>7, i16>>20, i32>>40, i64>>70, u64>>70)
	var m1 int64 = -1
	fmt.Println(i64/m1, i64%m1, int32(i64)/int32(m1), int8(-128)/int8(m1))
	for _, d := range []int64{1, -1, 3, -3, 7, -7} {
		fmt.Print(int64(-20)/d, " ", int64(-20)%d, " ", int64(20)/d, " ", int64(20)%d, "; ")
	}
	fmt.Println()
	n1, n2, b200, b31, b63 := int8(-1), int16(-2), uint8(200), uint32(1<<31), uint64(1<<63)
	fmt.Println(uint8(n1), int8(b200), uint16(n2), int32(b31), int64(b63), uint64(n1), int64(u32), int8(i32), uint8(i16))
}
