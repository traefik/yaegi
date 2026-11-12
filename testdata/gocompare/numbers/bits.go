// math/bits counting and reordering functions for every width: LeadingZeros,
// TrailingZeros, OnesCount, Len, RotateLeft, Reverse, ReverseBytes, on a
// table of values.
package main

import (
	"fmt"
	"math/bits"
)

func main() {
	fmt.Println(bits.UintSize == 32 || bits.UintSize == 64)
	for _, v := range []uint8{0, 1, 2, 0x0F, 0x80, 0xA5, 0xFF} {
		fmt.Println(v, bits.LeadingZeros8(v), bits.TrailingZeros8(v), bits.OnesCount8(v), bits.Len8(v),
			bits.RotateLeft8(v, 3), bits.RotateLeft8(v, -3), bits.Reverse8(v))
	}
	for _, v := range []uint16{0, 1, 0x00F0, 0x8000, 0xBEEF, 0xFFFF} {
		fmt.Println(v, bits.LeadingZeros16(v), bits.TrailingZeros16(v), bits.OnesCount16(v), bits.Len16(v),
			bits.RotateLeft16(v, 5), bits.RotateLeft16(v, -17), bits.Reverse16(v), bits.ReverseBytes16(v))
	}
	for _, v := range []uint32{0, 1, 0x80000000, 0xDEADBEEF, 0x00FF0000, 0xFFFFFFFF, 12345} {
		fmt.Println(v, bits.LeadingZeros32(v), bits.TrailingZeros32(v), bits.OnesCount32(v), bits.Len32(v),
			bits.RotateLeft32(v, 13), bits.RotateLeft32(v, -1), bits.Reverse32(v), bits.ReverseBytes32(v))
	}
	for _, v := range []uint64{0, 1, 1 << 63, 0xFEEDFACECAFEBEEF, 0x0000FFFF00000000, 1<<64 - 1, 1 << 40} {
		fmt.Println(v, bits.LeadingZeros64(v), bits.TrailingZeros64(v), bits.OnesCount64(v), bits.Len64(v),
			bits.RotateLeft64(v, 29), bits.RotateLeft64(v, -64), bits.Reverse64(v), bits.ReverseBytes64(v))
	}
	// uint versions with values that fit in 32 bits; results that do not
	// depend on the width of uint.
	for _, v := range []uint{0, 1, 6, 0x7FFFFFFF, 0x80000000} {
		fmt.Println(v, bits.TrailingZeros(v), bits.OnesCount(v), bits.Len(v), bits.LeadingZeros(v)-(bits.UintSize-32),
			uint32(bits.Reverse(v)>>(bits.UintSize-32)), uint32(bits.ReverseBytes(v)>>(bits.UintSize-32)),
			uint32(bits.RotateLeft(v, bits.UintSize-1)>>(bits.UintSize-32)))
	}
	// Loops over all bytes.
	sum := 0
	for i := 0; i < 256; i++ {
		b := uint8(i)
		sum += bits.OnesCount8(b) * bits.Len8(b)
		if bits.Reverse8(bits.Reverse8(b)) != b {
			fmt.Println("bad reverse", b)
		}
	}
	fmt.Println(sum)
}
