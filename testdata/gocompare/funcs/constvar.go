// const and var blocks: iota with skips, expressions and implicit
// repetition, typed and untyped constants, constant arithmetic beyond int64
// folded at compile time, var blocks with several names and types, zero
// values, and constants used as array sizes.
package main

import (
	"fmt"
	"math"
)

type Weekday int

const (
	Sunday Weekday = iota
	Monday
	Tuesday
	_
	Thursday
	Friday = iota * 10
	Saturday
)

func (d Weekday) String() string { return fmt.Sprintf("day%d", int(d)) }

const (
	_  = iota
	KB = 1 << (10 * iota)
	MB
	GB
	TB
)

const (
	a, b = iota, iota * 2
	c, d
	e, f = "e", iota
)

const big = 1 << 100
const small = big >> 98
const huge = big * big
const ratio = huge / big / big
const pi2 = math.Pi * 2
const typedF float32 = 1.0 / 3
const untypedF = 1.0 / 3
const str = "con" + "cat"
const ch = 'x' + 1
const boolC = len(str) > 3 && small == 4

type Flags uint8

const (
	FlagA Flags = 1 << iota
	FlagB
	FlagC
	FlagD
)

var (
	vi      int
	vs      string
	vp      *int
	vsl     []int
	vm      map[string]int
	vf      func()
	vif     interface{}
	vch     chan int
	vst     struct{ a, b int }
	va      [3]bool
	x, y, z       = 1, "two", 3.0
	w       int64 = 1 << 40
)

var arr [KB / 256]int

func main() {
	fmt.Println(Sunday, Monday, Tuesday, Thursday, Friday, Saturday)
	fmt.Println(KB, MB, GB, int64(TB))
	fmt.Println(a, b, c, d, e, f)
	fmt.Println(small, ratio, big/big)
	fmt.Println(float64(big) > 1e30, huge > big)
	fmt.Printf("%.6f %v %v\n", pi2, typedF, untypedF)
	fmt.Printf("%T %T %T %T\n", typedF, untypedF, ch, boolC)
	fmt.Println(str, ch, string(rune(ch)), boolC)
	fl := FlagA | FlagC
	fmt.Println(fl, fl&FlagB != 0, fl&FlagC != 0, ^fl, fl&^FlagA, FlagD)
	fmt.Println(vi, vs == "", vp == nil, vsl == nil, vm == nil, vf == nil, vif == nil, vch == nil, vst, va)
	fmt.Println(x, y, z, w)
	fmt.Printf("%T %T %T\n", x, y, z)
	fmt.Println(len(arr))
	const local = 7
	var la [local * 2]byte
	fmt.Println(len(la), cap(la[:local]))
	const mx = math.MaxInt64
	const mn = math.MinInt64
	fmt.Println(int64(mx), int64(mn), uint64(math.MaxUint64))
	const shift = 3
	var u uint64 = 1 << (64 - shift)
	fmt.Println(u)
	fmt.Println(math.MaxInt8, math.MinInt16, math.MaxUint16)
	var f32 float32 = 16777216.0
	fmt.Println(f32+1 == f32)
	const exact = 0.1 + 0.2
	fmt.Println(exact == 0.3, 0.1+0.2)
	v1, v2 := 0.1, 0.2
	fmt.Println(v1+v2 == 0.3)
	type E int
	const (
		E0 E = iota + 'A'
		E1
	)
	fmt.Println(E0, E1, string(rune(E1)))
}
