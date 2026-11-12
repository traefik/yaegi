// math/big.Int: arithmetic, Quo/Rem vs Div/Mod signs, Exp, GCD, ModInverse,
// bit operations, comparisons, SetString in several bases, output
// with String, Text and Append, and conversions to int64 and uint64. The
// fmt verbs of *big.Int (its Format method) are not printed here.
package main

import (
	"fmt"
	"math/big"
)

func main() {
	a, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
	b := big.NewInt(-987654321)
	z := new(big.Int)
	fmt.Println(z.Add(a, b), z.Sub(a, b), new(big.Int).Mul(a, b), new(big.Int).Neg(a), new(big.Int).Abs(b))
	for _, p := range [][2]int64{{7, 3}, {-7, 3}, {7, -3}, {-7, -3}, {6, 3}, {-1, 5}} {
		x, y := big.NewInt(p[0]), big.NewInt(p[1])
		q, r := new(big.Int).QuoRem(x, y, new(big.Int))
		d, m := new(big.Int).DivMod(x, y, new(big.Int))
		fmt.Println(p, q, r, d, m, new(big.Int).Quo(x, y), new(big.Int).Rem(x, y), new(big.Int).Div(x, y), new(big.Int).Mod(x, y))
	}
	two := big.NewInt(2)
	p := new(big.Int).Exp(two, big.NewInt(200), nil)
	fmt.Println(p, p.BitLen(), p.TrailingZeroBits(), new(big.Int).Exp(big.NewInt(3), big.NewInt(1000), big.NewInt(1000007)))
	fmt.Println(new(big.Int).Sqrt(p), new(big.Int).Lsh(a, 70), new(big.Int).Rsh(a, 33), new(big.Int).Rsh(big.NewInt(-5), 1))
	g := new(big.Int)
	x, y := new(big.Int), new(big.Int)
	g.GCD(x, y, big.NewInt(240), big.NewInt(46))
	fmt.Println(g, x, y, new(big.Int).ModInverse(big.NewInt(3), big.NewInt(11)), new(big.Int).ModInverse(big.NewInt(2), big.NewInt(4)))
	fmt.Println(new(big.Int).And(a, big.NewInt(0xFFFF)), new(big.Int).Or(b, big.NewInt(0xF0)), new(big.Int).Xor(a, b),
		new(big.Int).Not(b), new(big.Int).AndNot(a, big.NewInt(0xFF)), a.Bit(0), a.Bit(1), new(big.Int).SetBit(big.NewInt(0), 100, 1))
	fmt.Println(a.Cmp(b), b.Cmp(a), a.Cmp(a), a.CmpAbs(b), a.Sign(), b.Sign(), new(big.Int).Sign())
	fmt.Println(b.Int64(), b.IsInt64(), a.IsInt64(), big.NewInt(5).Uint64(), a.IsUint64(), new(big.Int).SetUint64(1<<64-1))
	// Bases.
	for _, s := range []string{"ff", "-0x1F", "0b1011", "0o777", "777", "z", "1_000", "", "12a"} {
		for _, base := range []int{0, 2, 8, 10, 16, 36} {
			v, ok := new(big.Int).SetString(s, base)
			if ok {
				fmt.Print(base, ":", v, " ")
			} else {
				fmt.Print(base, ":fail ")
			}
		}
		fmt.Println(s)
	}
	for _, base := range []int{2, 8, 10, 16, 36, 62} {
		fmt.Println(base, a.Text(base), b.Text(base))
	}
	fmt.Println(b.Text(16), a.Text(2), new(big.Int).Neg(a).Text(8), string(a.Append([]byte("x="), 16)))
	fmt.Println(a.String(), b.Append(nil, 10), len(a.Bytes()), new(big.Int).SetBytes([]byte{1, 0, 0}))
	// Factorial and binomial.
	f := new(big.Int).MulRange(1, 30)
	fmt.Println(f, new(big.Int).Binomial(50, 25), big.NewInt(97).ProbablyPrime(10), big.NewInt(91).ProbablyPrime(10))
	// Accumulate Fibonacci numbers.
	f0, f1 := big.NewInt(0), big.NewInt(1)
	for i := 0; i < 150; i++ {
		f0.Add(f0, f1)
		f0, f1 = f1, f0
	}
	fmt.Println(f0)
	// Float64 conversion.
	v, acc := a.Float64()
	fmt.Println(v, acc)
	var zero big.Int
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		fmt.Println(new(big.Int).Quo(a, &zero))
	}()
}
