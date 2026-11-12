// Shifts with counts below, equal to and above the width, with signed and
// unsigned count types, arithmetic right shift of negative values, and the
// panic of a negative shift count.
package main

import "fmt"

func try(name string, f func() string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(name, "panic:", r)
		}
	}()
	fmt.Println(name, f())
}

func main() {
	counts := []uint{0, 1, 7, 8, 15, 16, 31, 32, 63, 64, 65, 100, 255, 256, 1000}
	var i8 int8 = -5
	var u8 uint8 = 0xA5
	var i16 int16 = -12345
	var u16 uint16 = 0xBEEF
	var i32 int32 = -123456789
	var u32 uint32 = 0xDEADBEEF
	var i64 int64 = -1234567890123
	var u64 uint64 = 0xFEEDFACECAFEBEEF
	for _, c := range counts {
		fmt.Println(c, i8<<c, i8>>c, u8<<c, u8>>c, i16<<c, i16>>c, u16<<c, u16>>c)
		fmt.Println(c, i32<<c, i32>>c, u32<<c, u32>>c, i64<<c, i64>>c, u64<<c, u64>>c)
	}
	// Signed count types.
	for _, c := range []int8{0, 3, 9, 64, 127} {
		fmt.Print(u64>>c, " ", i64<<c, " ", u8<<c, "; ")
	}
	fmt.Println()
	for _, c := range []int64{0, 5, 40, 63, 64, 1 << 40} {
		fmt.Print(u32>>c, " ", i32>>c, " ", i16<<c, "; ")
	}
	fmt.Println()
	for _, c := range []uint64{0, 5, 1 << 33, 1 << 63} {
		fmt.Print(u16>>c, " ", i8>>c, " ", u64<<c, "; ")
	}
	fmt.Println()
	for _, c := range []uint8{0, 1, 33, 200} {
		fmt.Print(u32<<c, " ", i64>>c, "; ")
	}
	fmt.Println()
	// int and uint with small counts.
	var n int = 1
	var m uint = 3
	for c := 0; c < 31; c += 5 {
		fmt.Print(n<<c, " ", m<<c>>c, " ", -n<<c>>c, "; ")
	}
	fmt.Println()
	// Compound shifts.
	x := uint32(1)
	for k := 0; k < 5; k++ {
		x <<= 7
		fmt.Print(x, " ")
	}
	y := int64(-1)
	y <<= 62
	fmt.Println(y)
	y >>= 61
	fmt.Println(y)
	big := uint(200)
	y >>= big
	fmt.Println(y)
	// Untyped constant shifted by a variable takes the type of the context.
	var s uint = 33
	var r64 int64 = 1 << s
	var r32 int32 = 1 << (s - 2)
	var f = uint8(1) << (s - 30)
	fmt.Println(r64, r32, f)
	// Rotations written with shifts.
	for _, c := range []uint{0, 1, 13, 31, 32} {
		fmt.Print(u32<<c|u32>>(32-c), " ")
	}
	fmt.Println()
	neg := -1
	neg64 := int64(-3)
	try("shift<<neg", func() string { return fmt.Sprint(1 << neg) })
	try("shift>>neg", func() string { return fmt.Sprint(u64 >> neg64) })
	try("shift<<=neg", func() string { v := int8(3); v <<= neg; return fmt.Sprint(v) })
	try("shift ok", func() string { return fmt.Sprint(5 << (neg + 2)) })
}
