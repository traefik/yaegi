// math/big.Rat: normalization, arithmetic, comparisons, SetString with
// fractions and decimals, FloatString, RatString, String and Float64.
package main

import (
	"fmt"
	"math/big"
)

func main() {
	a := big.NewRat(6, -4)
	b := big.NewRat(1, 3)
	fmt.Println(a, b, a.Num(), a.Denom(), a.IsInt(), big.NewRat(4, 2).IsInt())
	fmt.Println(new(big.Rat).Add(a, b), new(big.Rat).Sub(a, b), new(big.Rat).Mul(a, b), new(big.Rat).Quo(a, b),
		new(big.Rat).Neg(a), new(big.Rat).Abs(a), new(big.Rat).Inv(b))
	fmt.Println(a.Cmp(b), b.Cmp(a), a.Sign(), new(big.Rat).Sign())
	for _, s := range []string{"3/4", "-10/15", "1.25", "-0.001", "1e3", "1e-3", "0x10", "1/0", "abc", "2.5e-2", "007"} {
		r, ok := new(big.Rat).SetString(s)
		if ok {
			fmt.Println(s, r, r.RatString(), r.FloatString(4))
		} else {
			fmt.Println(s, "fail")
		}
	}
	third := big.NewRat(1, 3)
	for _, n := range []int{0, 1, 5, 20} {
		fmt.Print(third.FloatString(n), " ", big.NewRat(-2, 3).FloatString(n), " ", big.NewRat(5, 2).FloatString(n), "; ")
	}
	fmt.Println()
	f, exact := big.NewRat(1, 3).Float64()
	g, exact2 := big.NewRat(3, 4).Float64()
	h, exact3 := big.NewRat(1, 10).Float32()
	fmt.Println(f, exact, g, exact2, h, exact3)
	r := new(big.Rat).SetFloat64(0.1)
	fmt.Println(r, new(big.Rat).SetInt64(-7), new(big.Rat).SetFrac64(10, 4), new(big.Rat).SetFrac(big.NewInt(9), big.NewInt(-12)))
	// Harmonic sum exactly.
	sum := new(big.Rat)
	for i := int64(1); i <= 20; i++ {
		sum.Add(sum, big.NewRat(1, i))
	}
	fmt.Println(sum, sum.FloatString(10))
	fmt.Println(sum.String(), b.String(), a.Num().String(), a.Denom().Text(2))
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		fmt.Println(big.NewRat(1, 0))
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		fmt.Println(new(big.Rat).Quo(a, new(big.Rat)))
	}()
}
