// Constants and iota patterns: skipped values, expressions, bit flags, byte
// sizes, typed iota, several constants per line, and String methods.
package main

import "fmt"

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

func (d Weekday) String() string { return fmt.Sprintf("Day(%d)", int(d)) }

type Flag uint8

const (
	FlagA Flag = 1 << iota
	FlagB
	FlagC
	FlagD
	FlagAll = FlagA | FlagB | FlagC | FlagD
)

const (
	_  = iota
	KB = 1 << (10 * iota)
	MB
	GB
	TB
	PB
	EB
)

const (
	a, b = iota, iota * 2
	c, d
	e, f = iota + 100, -iota
	g, h
)

const (
	x0 = "s"
	x1
	x2 = iota
	x3
)

const (
	m0 = iota % 3
	m1
	m2
	m3
	m4
)

type Level int8

const (
	Debug Level = iota - 2
	Info
	Warn
	Error
)

const (
	f0 = 1.5 * iota
	f1
	f2
)

const (
	big    = 1 << 100
	bigger = big * big
	small  = bigger >> 198
)

func main() {
	fmt.Println(Sunday, Monday, Tuesday, Thursday, Friday, Saturday)
	fmt.Printf("%d %v %s\n", Saturday, Saturday, Monday)
	fmt.Println(FlagA, FlagB, FlagC, FlagD, FlagAll, FlagAll&^FlagB, FlagAll^FlagC)
	var fl Flag = FlagA | FlagC
	fmt.Println(fl&FlagC != 0, fl&FlagB != 0, ^fl, fl<<4)
	fmt.Println(KB, MB, GB, int64(TB), int64(PB), uint64(EB))
	fmt.Println(a, b, c, d, e, f, g, h)
	fmt.Println(x0, x1, x2, x3)
	fmt.Println(m0, m1, m2, m3, m4)
	fmt.Println(Debug, Info, Warn, Error)
	fmt.Printf("%T %v\n", Warn, Error-Debug)
	fmt.Println(f0, f1, f2)
	fmt.Printf("%T %T\n", f1, a)
	fmt.Println(small, big/(1<<98), float64(big))
	const huge = 1e400
	fmt.Println(huge/1e399, float32(huge/1e370))
	const third = 1.0 / 3
	fmt.Println(third, float32(third), third*3 == 1)
	const r = 'a' + 1
	fmt.Printf("%T %v %c\n", r, r, r)
	const s = len("hello") * 2
	var arr [s]int
	fmt.Println(len(arr), cap(arr))
	// Constant expressions with mixed kinds.
	const ci = 7 / 2
	const cf = 7 / 2.0
	const cm = 7 % 3
	const cc = 1 + 2i
	fmt.Println(ci, cf, cm, cc*cc, real(cc))
	// Typed constants.
	const t8 int8 = -128
	const tu uint16 = 65535
	fmt.Println(t8, tu, t8/2, tu>>8, -(t8 + 1))
	// iota inside a function.
	const (
		p = iota * iota
		q
		rr
		ss
	)
	fmt.Println(p, q, rr, ss)
	// iota with typed shift.
	type Bits uint64
	const (
		B0 Bits = 1 << (iota * 16)
		B1
		B2
		B3
	)
	fmt.Println(B0, B1, B2, B3, B3>>48)
}
