// math/bits arithmetic with carry and overflow: Add, Sub, Mul, Div, Rem for
// 32 and 64 bits, and the panics of Div by zero and Div overflow.
package main

import (
	"fmt"
	"math/bits"
)

func try(name string, f func() string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(name, "panic:", r)
		}
	}()
	fmt.Println(name, f())
}

func main() {
	u32 := []uint32{0, 1, 0x7FFFFFFF, 0x80000000, 0xFFFFFFFE, 0xFFFFFFFF, 123456789}
	for _, a := range u32 {
		for _, b := range u32 {
			s, c := bits.Add32(a, b, 1)
			d, br := bits.Sub32(a, b, 1)
			hi, lo := bits.Mul32(a, b)
			fmt.Print(s, c, " ", d, br, " ", hi, lo, "|")
		}
		fmt.Println()
	}
	u64 := []uint64{0, 1, 1<<63 - 1, 1 << 63, 1<<64 - 2, 1<<64 - 1, 0x123456789ABCDEF0}
	for _, a := range u64 {
		for _, b := range u64 {
			s, c := bits.Add64(a, b, 0)
			s1, c1 := bits.Add64(a, b, 1)
			d, br := bits.Sub64(a, b, 0)
			d1, br1 := bits.Sub64(a, b, 1)
			hi, lo := bits.Mul64(a, b)
			fmt.Print(s, c, s1, c1, " ", d, br, d1, br1, " ", hi, lo, "|")
		}
		fmt.Println()
	}
	// 128-bit by 64-bit division.
	for _, t := range [][3]uint64{{0, 100, 7}, {1, 0, 3}, {5, 1<<64 - 1, 1<<64 - 1}, {0, 1<<64 - 1, 1 << 32}, {123, 456, 789}} {
		q, r := bits.Div64(t[0], t[1], t[2])
		fmt.Println(t, q, r, bits.Rem64(t[0], t[1], t[2]))
	}
	for _, t := range [][3]uint32{{0, 100, 7}, {1, 0, 3}, {2, 5, 0xFFFFFFFF}} {
		q, r := bits.Div32(t[0], t[1], t[2])
		fmt.Println(t, q, r, bits.Rem32(t[0], t[1], t[2]))
	}
	fmt.Println(bits.Rem64(10, 5, 3), bits.Rem32(10, 5, 3))
	var zero uint64
	var z32 uint32
	try("Div64 by zero", func() string { q, r := bits.Div64(0, 1, zero); return fmt.Sprint(q, r) })
	try("Div64 overflow", func() string { q, r := bits.Div64(5, 1, 5); return fmt.Sprint(q, r) })
	try("Div32 by zero", func() string { q, r := bits.Div32(0, 1, z32); return fmt.Sprint(q, r) })
	try("Div32 overflow", func() string { q, r := bits.Div32(9, 1, 3); return fmt.Sprint(q, r) })
	try("Rem64 by zero", func() string { return fmt.Sprint(bits.Rem64(1, 1, zero)) })
	// A 128-bit multiply-add chain built from Mul64 and Add64.
	var hi, lo uint64 = 0, 1
	for i := 0; i < 20; i++ {
		h1, l1 := bits.Mul64(lo, 0xFFFFFFFFFFFFFFC5)
		_, l2 := bits.Mul64(hi, 0xFFFFFFFFFFFFFFC5)
		var c uint64
		lo, c = bits.Add64(l1, 12345, 0)
		hi, _ = bits.Add64(h1, l2, c)
	}
	fmt.Println(hi, lo)
}
