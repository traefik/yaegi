// Generic numeric code: sums, products, conversions and comparisons over
// type parameters with constraints of integers, floats and complex, with
// wrapping at the width of each type argument.
package main

import (
	"fmt"
	"math"
)

type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

type Float interface{ ~float32 | ~float64 }

type Number interface {
	Integer | Float
}

func Sum[T Number](xs ...T) T {
	var s T
	for _, x := range xs {
		s += x
	}
	return s
}

func Product[T Integer](xs ...T) T {
	p := T(1)
	for _, x := range xs {
		p *= x
	}
	return p
}

func Convert[To, From Number](v From) To { return To(v) }

func MaxOf[T Integer]() T {
	var z T
	z--
	if z > 0 {
		return z
	}
	return T(1)<<(8*sizeOf(z)-1) - 1
}

func sizeOf[T Integer](v T) int {
	n := 0
	for x := ^T(0); x != 0; x <<= 1 {
		n++
	}
	_ = v
	return (n + 7) / 8
}

func Div[T Integer](a, b T) (q T, r T, err any) {
	defer func() { err = recover() }()
	return a / b, a % b, nil
}

func Shl[T Integer](v T, n uint) T { return v << n }
func Shr[T Integer](v T, n uint) T { return v >> n }
func Neg[T Number](v T) T          { return -v }

func Abs[T Float](v T) T {
	if v < 0 {
		return -v
	}
	return v
}

type MyInt int16
type MyFloat float32

func Cplx[T ~complex64 | ~complex128](a, b T) (T, T, T) { return a + b, a * b, a / b }

func main() {
	fmt.Println(Sum[int8](100, 27, 1), Sum[uint8](200, 100), Sum(1.5, 2.25), Sum[float32](0.1, 0.2), Sum[MyInt](32767, 1))
	fmt.Println(Product[int16](300, 300), Product[uint32](65536, 65536, 3), Product[int64](1<<31, 1<<31, 4), Product[uint8](16, 16))
	fmt.Println(Convert[int8](300), Convert[uint8, int64](-1), Convert[float32](16777217), Convert[int32](2.9), Convert[uint16, float64](65535.5))
	fmt.Println(Convert[float64](int64(math.MaxInt64)), Convert[MyFloat](MyInt(-7)), Convert[int64, MyFloat](MyFloat(-2.75)))
	fmt.Println(MaxOf[int8](), MaxOf[uint8](), MaxOf[int16](), MaxOf[uint16](), MaxOf[int32](), MaxOf[uint32](), MaxOf[int64](), MaxOf[uint64](), MaxOf[MyInt]())
	fmt.Println(sizeOf(int8(0)), sizeOf(uint16(0)), sizeOf(int32(0)), sizeOf(uint64(0)))
	fmt.Println(Div[int8](-128, -1))
	fmt.Println(Div[int64](math.MinInt64, -1))
	fmt.Println(Div[uint16](7, 0))
	fmt.Println(Div[int32](-7, 2))
	fmt.Println(Shl[int8](1, 7), Shl[uint8](1, 8), Shr[int16](-32768, 15), Shr[int16](-32768, 100), Shl[uint64](3, 63), Shr[int32](-1, 31))
	fmt.Println(Neg[int8](-128), Neg[uint8](1), Neg[float64](0), Neg[MyInt](5), Abs[float32](-2.5), Abs(MyFloat(-1)))
	fmt.Println(Cplx[complex64](1+2i, 3-1i))
	fmt.Println(Cplx(complex128(2i), 1+1i))
	fmt.Printf("%T %T %T\n", Sum[MyInt](1), Convert[MyFloat](1), Neg[MyFloat](1))
}
