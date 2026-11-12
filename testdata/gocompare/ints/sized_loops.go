// Loop counters of small integer types wrap: loops that stop on wrap, the
// sum and xor of all values of uint8 and int8, and a checksum computed
// with uint16 and uint32 arithmetic.
package main

import "fmt"

func main() {
	n := 0
	for i := uint8(250); i != 4; i++ {
		n++
	}
	fmt.Println(n)
	var s16 int16
	var x8 int8
	for i := int8(-128); ; i++ {
		s16 += int16(i)
		x8 ^= i
		if i == 127 {
			break
		}
	}
	fmt.Println(s16, x8)
	var su8 uint8
	var su32 uint32
	for i := 0; i < 256; i++ {
		su8 += uint8(i)
		su32 += uint32(i) * uint32(i) * 16777619
	}
	fmt.Println(su8, su32)
	// Fletcher-16 and FNV-1a 32 and 64 over a fixed input.
	data := []byte("The quick brown fox jumps over the lazy dog 0123456789")
	var a, b uint16
	for _, d := range data {
		a = (a + uint16(d)) % 255
		b = (b + a) % 255
	}
	fmt.Println(b<<8 | a)
	h32 := uint32(2166136261)
	h64 := uint64(14695981039346656037)
	for _, d := range data {
		h32 ^= uint32(d)
		h32 *= 16777619
		h64 ^= uint64(d)
		h64 *= 1099511628211
	}
	fmt.Println(h32, h64)
	// Down-counting unsigned loop.
	cnt := 0
	for u := uint16(5); u < 10; u-- {
		cnt++
	}
	fmt.Println(cnt)
	// LCG with wrap.
	var seed uint32 = 12345
	for i := 0; i < 5; i++ {
		seed = seed*1103515245 + 12345
		fmt.Print(seed>>16&0x7fff, " ")
	}
	fmt.Println()
	var x64 uint64 = 88172645463325252
	for i := 0; i < 5; i++ {
		x64 ^= x64 << 13
		x64 ^= x64 >> 7
		x64 ^= x64 << 17
		fmt.Print(x64, " ")
	}
	fmt.Println()
	// int8 loop counter with step.
	for i := int8(100); i > 0 && i < 127; i += 10 {
		fmt.Print(i, " ")
	}
	fmt.Println()
}
