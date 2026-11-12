// The inline paths of shifts and numeric conversions: shifts by a signed
// count, by an unsigned count above MaxInt64, int/float conversions of each
// kind, and numeric constants of each type (also in generic code).
package main

import (
	"fmt"
	"math"
)

type celsius float64
type small float32

func hash(s string) uint32 {
	var h uint32
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i]) << (i % 4 * 8)
		h = h<<5 | h>>27
	}
	return h
}

func shl(x, s int) (r int, err any) {
	defer func() { err = recover() }()
	return x << s, nil
}

func shr(x int64, s int8) (r int64, err any) {
	defer func() { err = recover() }()
	return x >> s, nil
}

func shru(x uint16, s int32) (r uint16, err any) {
	defer func() { err = recover() }()
	return x >> s, nil
}

func ushifts(x int64, u uint64) (int64, int64, uint64) {
	return x << u, x >> u, uint64(x) >> u
}

func toF[T ~int8 | ~int16 | ~int32 | ~int64 | ~int | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uint](x T) (float64, float32) {
	return float64(x), float32(x)
}

func addOne[T ~int | ~float64 | ~float32](x T) T { return x + 1.0 }

func main() {
	fmt.Println(hash("go-vm"), hash("hello, world"))
	fmt.Println(shl(3, 4))
	fmt.Println(shl(3, 70))
	fmt.Println(shl(3, -1))
	fmt.Println(shr(-256, 4))
	fmt.Println(shr(-256, -4))
	fmt.Println(shru(65535, 3))
	fmt.Println(shru(65535, -3))
	fmt.Println(ushifts(-5, 1<<63))
	fmt.Println(ushifts(-5, 2))

	// Integer to float, for each kind.
	fmt.Println(toF(int8(-128)))
	fmt.Println(toF(int16(-32768)))
	fmt.Println(toF(int32(math.MinInt32)))
	fmt.Println(toF(int64(math.MinInt64)))
	fmt.Println(toF(int(1<<53 + 1)))
	fmt.Println(toF(uint8(255)))
	fmt.Println(toF(uint16(65535)))
	fmt.Println(toF(uint32(math.MaxUint32)))
	fmt.Println(toF(uint64(math.MaxUint64)))
	fmt.Println(toF(uint(1<<63 + 1)))
	n := 16777217
	fmt.Println(float64(n), float32(n), celsius(n), small(n))

	// Float to integer, for each kind.
	for _, f := range []float64{-2.5, 0.9, 127.9, 254.5, 4e9, -9.2e18} {
		fmt.Println(int(f), int64(f), int8(f), int16(f), int32(f))
		if f >= 0 {
			fmt.Println(uint(f), uint8(f), uint16(f), uint32(f), uint64(f))
		}
		g := float32(f)
		fmt.Println(int(g), int64(g), int8(g), int32(g))
	}
	var c celsius = 36.6
	fmt.Println(int(c), int64(c), uint8(c))

	// Constants of each numeric type.
	const big uint64 = 1<<64 - 1
	var f32 float32 = 0.1
	var cs celsius = 1 << 3
	var sm small = 1.0 / 3
	fmt.Println(big, f32, float64(f32), cs, sm, float64(sm))
	fmt.Println(addOne(41), addOne(0.5), addOne(float32(0.1)), addOne(celsius(-1)))
}
