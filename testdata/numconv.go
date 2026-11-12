// Integer and float conversions for every kind: wrap-around on overflow,
// sign extension and truncation, shifts by large and signed counts, division
// and remainder of negative numbers, MinInt / -1, float to int of in-range
// values, unsigned conversions of negative values, constants, and the
// min, max and clear builtins.
package main

import (
	"fmt"
	"math"
	"math/bits"
	"strconv"
)

func two() int { return 2 }

type MyInt8 int8
type MyUint uint

func main() {
	vals := []int64{0, 1, -1, 127, 128, -128, -129, 255, 256, 32767, 32768, -32769, 65535, 65536,
		math.MaxInt32, math.MinInt32, math.MaxInt32 + 1, math.MaxUint32, math.MaxInt64, math.MinInt64, 0x123456789abcdef0}
	for _, v := range vals {
		fmt.Println(v, int8(v), int16(v), int32(v), uint8(v), uint16(v), uint32(v), uint64(v), MyInt8(v), uintptr(v))
	}

	// Arithmetic that wraps.
	var i8 int8 = 127
	i8++
	var u8 uint8 = 0
	u8--
	var i16 int16 = -32768
	i16 = -i16
	var i32 int32 = math.MaxInt32
	i32 += 2
	var u32 uint32 = 1 << 31
	u32 *= 2
	var i64 int64 = math.MaxInt64
	i64++
	var u64 uint64 = 0
	u64 -= 3
	fmt.Println(i8, u8, i16, i32, u32, i64, u64)

	var m8 MyInt8 = -128
	m8 *= -1
	fmt.Println(m8, m8/-1)
	var mu MyUint = 3
	mu -= 5
	fmt.Println(mu == math.MaxUint-1)

	// MinInt / -1 and % -1 do not panic in Go.
	minv := int64(math.MinInt64)
	neg := int64(-1)
	fmt.Println(minv/neg, minv%neg)
	min32 := int32(math.MinInt32)
	fmt.Println(min32/int32(neg), min32%int32(neg))
	min8 := int8(-128)
	fmt.Println(min8/int8(neg), min8%int8(neg))

	// Division and remainder truncate toward zero.
	for _, p := range [][2]int{{7, 2}, {-7, 2}, {7, -2}, {-7, -2}} {
		fmt.Print(p[0]/p[1], " ", p[0]%p[1], "; ")
	}
	fmt.Println()

	// Shifts: counts larger than the width, signed counts, arithmetic
	// shift of negative values.
	x := int32(-8)
	for _, s := range []uint{0, 1, 3, 31, 32, 33, 64, 100} {
		fmt.Print(x>>s, " ", uint32(x)>>s, " ", x<<s, " ", int8(1)<<s, "; ")
	}
	fmt.Println()
	sc := 3
	fmt.Println(1<<sc, -16>>sc, uint8(255)<<sc)
	func() {
		defer func() { fmt.Println("negative shift:", recover()) }()
		n := -1
		fmt.Println(1 << n)
	}()

	// Float conversions.
	for _, f := range []float64{0, -0.0, 1.9, -1.9, 2.5, -2.5, 1e10, 255.99, 3.999999} {
		fmt.Print(int64(f), " ", int32(int64(f)), " ", uint8(int64(f)), " ", float32(f), "; ")
	}
	fmt.Println()
	f32 := float32(16777217) // not exact in float32
	fmt.Println(f32, float64(f32), int(f32))
	fmt.Println(float32(0.1)+float32(0.2), 0.1+0.2, float32(1)/3)
	var zero float64
	fmt.Println(1/zero, -1/zero, zero/zero == zero/zero, math.Signbit(-zero), math.Copysign(0, -1))
	fmt.Println(math.MaxFloat32, math.SmallestNonzeroFloat64, float32(math.MaxFloat32)*float32(two()))
	fmt.Println(uint64(float64(1<<63)), int64(float64(-1<<63)))

	// Constants and typed constants.
	const big = 1 << 100
	fmt.Println(big>>98, float64(big), uint8(big>>93))
	const typed int16 = 1000
	fmt.Println(typed*30, int32(typed)*100)

	// bits.
	fmt.Println(bits.OnesCount64(math.MaxUint64), bits.LeadingZeros32(1), bits.Reverse8(1), bits.Len(255))
	hi, lo := bits.Mul64(math.MaxUint64, 2)
	sum, carry := bits.Add64(math.MaxUint64, 1, 0)
	fmt.Println(hi, lo, sum, carry)

	// min, max, clear.
	fmt.Println(min(3, 1, 2), max(3, 1, 2), min(2.5, -1.0), max("b", "a", "c"), min(int8(-3), 4))
	fmt.Println(min(math.NaN(), 1), max(1, math.NaN()), min(-0.0, 0.0), max(-0.0, 0.0))
	fmt.Println(math.Signbit(min(-0.0, 0.0)), math.Signbit(max(-0.0, 0.0)))
	s := []int{1, 2, 3}
	sub := s[:2]
	clear(sub)
	fmt.Println(s, len(sub))
	mp := map[string]int{"a": 1, "b": 2}
	alias := mp
	clear(mp)
	fmt.Println(len(alias), alias)

	// strconv on all widths.
	fmt.Println(strconv.FormatInt(-129, 16), strconv.FormatUint(math.MaxUint64, 36))
	_, err := strconv.ParseInt("128", 10, 8)
	fmt.Println(err)
	v, err := strconv.ParseInt("-0x80", 0, 8)
	fmt.Println(v, err)
	u, err := strconv.ParseUint("1_000", 0, 16)
	fmt.Println(u, err)

	// Complex.
	c := complex(1, 2)
	c *= c
	fmt.Println(c, real(c), imag(c), complex64(c))

	// Printf verbs on numbers.
	fmt.Printf("%d %x %X %o %O %b %c %U %q\n", 255, 255, 255, 8, 8, 5, 'G', 'G', 'G')
	fmt.Printf("%08.3f|%-8d|%+d|% d|%e|%g|%.2g\n", math.Pi, 42, 42, 42, 1234.5678, 1e21, 1234.5678)
	fmt.Printf("%x %X % x\n", -255, uint8(200), []byte{1, 171})
	fmt.Printf("%v %d %v\n", MyInt8(-5), MyUint(7), []MyInt8{1, -1})
	fmt.Printf("%t %v %p\n", true, nil, nil)
	fmt.Printf("%6.2f%%|%*d|%-*d|\n", 99.5, 5, 42, 4, 7)
}
