// Comparisons of unsigned integers used as values (ok := a >= b), not as
// the condition of a branch: equal operands, smaller and larger ones, and
// values with the high bit set, which a signed comparison would order
// the other way.
package main

import "fmt"

//go:noinline
func pair32(a, b uint32) (uint32, uint32) { return a, b }

func cmp32(a, b uint32) {
	lt := a < b
	le := a <= b
	gt := a > b
	ge := a >= b
	fmt.Println("uint32", a, b, lt, le, gt, ge)
}

func cmp64(a, b uint64) {
	lt := a < b
	le := a <= b
	gt := a > b
	ge := a >= b
	fmt.Println("uint64", a, b, lt, le, gt, ge)
}

func cmp8(a, b uint8) {
	ge := a >= b
	le := a <= b
	fmt.Println("uint8", a, b, ge, le)
}

func cmpU(a, b uint) {
	ge := a >= b
	gt := a > b
	fmt.Println("uint", a, b, ge, gt)
}

func cmpPtr(a, b uintptr) {
	ge := a >= b
	lt := a < b
	fmt.Println("uintptr", a, b, ge, lt)
}

func main() {
	a, b := pair32(7, 7)
	ok := a >= b
	fmt.Println(ok)
	cmp32(7, 7)
	cmp32(6, 7)
	cmp32(8, 7)
	cmp32(1<<31, 1)
	cmp32(1, 1<<31)
	cmp32(0, 0)
	cmp32(1<<32-1, 1<<32-1)

	cmp64(7, 7)
	cmp64(6, 7)
	cmp64(1<<63, 1)
	cmp64(1, 1<<63)
	cmp64(1<<64-1, 1<<63)

	cmp8(200, 200)
	cmp8(200, 100)
	cmp8(100, 200)

	cmpU(7, 7)
	cmpU(1<<31, 3)
	cmpU(3, 1<<31)

	cmpPtr(7, 7)
	cmpPtr(1<<31, 1)

	// in a slice of results, as the value of a function, and in a map
	vs := []uint32{0, 7, 1 << 31, 1<<32 - 1}
	var res []bool
	for _, x := range vs {
		for _, y := range vs {
			res = append(res, x >= y)
		}
	}
	fmt.Println(res)
	ge := func(x, y uint16) bool { return x >= y }
	fmt.Println(ge(7, 7), ge(1<<15, 1), ge(1, 1<<15))
	m := map[bool]int{}
	for _, x := range vs {
		m[x >= 7]++
	}
	fmt.Println(m[true], m[false])
}
