// Typed and untyped constants: default types, conversions at assignment,
// exact constant arithmetic, rune and float constants, and the type of
// mixed expressions with variables.
package main

import (
	"fmt"
	"math"
)

type Meters float64
type Small int8

func show(v any) { fmt.Printf("%T=%v ", v, v) }

func main() {
	show(1)
	show(1.0)
	show('x')
	show(1i)
	show(1 << 10)
	show(2.0 * 3)
	show('a' * 2)
	show(1 + 1.5)
	show(1 + 'a')
	show(3 / 2)
	show(3 / 2.0)
	show(float32(1) / 3)
	show(int64(1) << 40)
	show(math.Pi)
	show(math.MaxInt32)
	show(uint64(math.MaxUint64))
	fmt.Println()
	var m Meters = 3
	m = Meters(m * 2) // the conversion rounds: gc may not fuse with the next +=
	m += 1.5
	show(m)
	show(m / 4)
	var s Small = 100
	s = s + 27
	show(s)
	show(s + 1)
	fmt.Println()
	// Exact constant arithmetic before conversion.
	const big = 1 << 62
	const k = big * 4 / 8
	show(int64(k))
	const fl = 0.1 + 0.2
	show(fl)
	show(fl == 0.3)
	x, y := 0.1, 0.2
	show(x+y == 0.3)
	show(float32(fl))
	const e = 1e-300 * 1e-300 * 1e600
	show(e)
	fmt.Println()
	// Shifts of untyped constants in float context.
	var u uint = 2
	var i64 int64 = 1 << u
	var f32 = float32(int32(1) << u)
	show(i64)
	show(f32)
	// Constant expressions with typed operands.
	const t int16 = 1000
	show(t * 30)
	show(t / 3 * 3)
	show(-t)
	var v8 int8 = 5
	show(v8 * 25)
	show(v8 * -25)
	fmt.Println()
	// Rune and byte constants.
	const ch = 'Z'
	var bb byte = ch
	var rr rune = ch + 1
	show(bb)
	show(rr)
	show(string(rr))
	show(ch - 'A')
	fmt.Println()
	// Untyped bool and string constants.
	const ok = 1 < 2
	type B bool
	var mb B = ok
	show(mb)
	const str = "abc" + "def"
	type S string
	var ms S = str
	show(ms)
	show(len(str))
	fmt.Println()
	// Float constants into integer variables when exact.
	var n int = 1e3
	var q int64 = 2.5e10
	var w uint8 = 'A' + 0.0
	show(n)
	show(q)
	show(w)
	const cplx = 2 + 0i
	var ci int = cplx
	show(ci)
	fmt.Println()
	// Constants mixing math constants.
	show(float32(math.MaxFloat32))
	show(math.SmallestNonzeroFloat32)
	show(int64(math.MinInt64))
	show(math.MaxUint16)
	show(math.Sqrt2 * math.Sqrt2)
	fmt.Println()
}
