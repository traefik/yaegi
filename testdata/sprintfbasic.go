package main

import (
	"errors"
	"fmt"
	"math"
)

// fmt.Sprintf, Sprint and Sprintln with arguments of predeclared basic
// types take a direct call (vm/natfast.go): the dynamic types (%T), the
// values of each size and the spacing rules of Sprint must be the ones of
// Go. Other arguments take the general path.

type level int

func (l level) String() string { return fmt.Sprintf("level-%d", int(l)) }

type plain int

func main() {
	var (
		b    = true
		i    = -42
		i8   = int8(-8)
		i16  = int16(-16)
		i32  = int32(-32)
		i64  = int64(math.MinInt64)
		u    = uint(42)
		u8   = uint8(255)
		u16  = uint16(65535)
		u32  = uint32(math.MaxUint32)
		u64  = uint64(math.MaxUint64)
		up   = uintptr(0xdead)
		f32  = float32(1.25)
		f64  = 3.141592653589793
		s    = "héllo"
		e    = ""
		r    = 'é'
		by   = byte('x')
		none any
	)
	fmt.Println(fmt.Sprintf("%v %v %v %v %v %v %v", b, i, i8, i16, i32, i64, u))
	fmt.Println(fmt.Sprintf("%v %v %v %v %v %#x", u8, u16, u32, u64, up, up))
	fmt.Println(fmt.Sprintf("%v %.3f %08.3f %g %e", f32, f32, f64, f64, f64))
	fmt.Println(fmt.Sprintf("%q %q %c %c %U %d", s, e, r, by, r, by))
	fmt.Println(fmt.Sprintf("%T %T %T %T %T %T %T %T %T %T %T %T %T %T %T %T", b, i, i8, i16, i32, i64, u, u8, u16, u32, u64, up, f32, f64, s, none))
	fmt.Println(fmt.Sprintf("%v|%d|%s", none, none, none))
	fmt.Println(fmt.Sprintf("%d %d", 1))
	fmt.Println(fmt.Sprintf("%d", 1, 2))
	fmt.Println(fmt.Sprintf("no verbs"))
	fmt.Println(fmt.Sprintf("%5s|%-5s|%x|%X", "ab", "cd", "hi", 255))
	fmt.Println(fmt.Sprintf("%t %v", false, math.Inf(-1)))
	fmt.Println(fmt.Sprintf("%v %v", float32(math.NaN()), -0.0))

	// Sprint adds spaces between operands when neither is a string.
	fmt.Println(fmt.Sprint(1, 2, "a", "b", 3, 4.5, true, none))
	fmt.Println(fmt.Sprint())
	fmt.Println(fmt.Sprintln("x", 1, 2, u8, f32) + "|")

	// Arguments with methods, named types and composites: general path.
	fmt.Println(fmt.Sprintf("%v %d %v", level(3), level(3), plain(7)))
	fmt.Println(fmt.Sprintf("%v %v %v", errors.New("boom"), []int{1, 2}, map[string]int{"a": 1}))
	fmt.Println(fmt.Sprintf("%v %T", complex(1, 2), complex64(complex(1, 2))))
	fmt.Println(fmt.Sprint(level(1), 2, plain(3)))
	args := []any{1, "two", 3.0}
	fmt.Println(fmt.Sprintf("%v-%v-%v", args...))
	args = append(args, level(4))
	fmt.Println(fmt.Sprint(args...))
}
