// math/big.Float: precision, rounding modes, arithmetic, Sqrt, comparisons,
// Text with each format and precision, String, Parse, and Int and Int64
// conversions. A *big.Float is printed with String or Text, not with fmt verbs.
package main

import (
	"fmt"
	"math"
	"math/big"
)

func main() {
	x := new(big.Float).SetPrec(200)
	x.SetInt64(2)
	s := new(big.Float).SetPrec(200).Sqrt(x)
	fmt.Println(s.Text('g', 50), s.Prec(), s.Acc())
	a := big.NewFloat(1.5)
	b := big.NewFloat(-0.25)
	fmt.Println(new(big.Float).Add(a, b).String(), new(big.Float).Sub(a, b).String(), new(big.Float).Mul(a, b).String(), new(big.Float).Quo(a, b).String(), new(big.Float).Neg(b).String(), new(big.Float).Abs(b).String())
	fmt.Println(a.Cmp(b), b.Cmp(a), a.Sign(), b.Signbit(), a.IsInt(), big.NewFloat(3).IsInt(), a.MinPrec(), a.Mode())
	for _, mode := range []big.RoundingMode{big.ToNearestEven, big.ToNearestAway, big.ToZero, big.AwayFromZero, big.ToNegativeInf, big.ToPositiveInf} {
		v := new(big.Float).SetPrec(4).SetMode(mode)
		v.SetFloat64(2.6875)
		w := new(big.Float).SetPrec(4).SetMode(mode)
		w.SetFloat64(-2.6875)
		fmt.Println(mode, v.String(), v.Text('g', -1), v.Acc(), w.Text('f', 3), w.Acc())
	}
	third := new(big.Float).SetPrec(100).Quo(big.NewFloat(1), big.NewFloat(3))
	for _, f := range []byte{'e', 'E', 'f', 'g', 'G', 'b', 'p', 'x'} {
		for _, p := range []int{-1, 0, 3, 10} {
			fmt.Print(third.Text(f, p), " ")
		}
		fmt.Println()
	}
	big1 := new(big.Float).SetPrec(300)
	big1.SetString("1e100")
	fmt.Println(big1.Text('e', 20), big1.Text('f', 0), big1.Text('g', -1))
	i, acc := big1.Int(nil)
	fmt.Println(i, acc)
	for _, v := range []float64{2.5, -2.5, 1e18, -0.75} {
		n, acc := big.NewFloat(v).Int64()
		u, acc2 := big.NewFloat(v).Uint64()
		fmt.Println(v, n, acc, u, acc2)
	}
	f64, acc3 := third.Float64()
	f32, acc4 := third.Float32()
	fmt.Println(f64, acc3, f32, acc4)
	for _, str := range []string{"1.5", "-0x1p-3", "1e-10", "inf", "-Inf", "0", "-0", "12abc", "0b101.1"} {
		v, base, err := big.ParseFloat(str, 0, 64, big.ToNearestEven)
		if err != nil {
			fmt.Println(str, "error:", err)
			continue
		}
		fmt.Println(str, v.String(), base, v.IsInf(), v.Signbit())
	}
	inf := new(big.Float).SetInf(false)
	fmt.Println(inf.String(), new(big.Float).SetInf(true).String(), inf.IsInf(), new(big.Float).Add(inf, a).String())
	func() {
		defer func() {
			r := recover()
			if e, ok := r.(big.ErrNaN); ok {
				fmt.Println("ErrNaN:", e.Error())
			} else {
				fmt.Println("recovered:", r)
			}
		}()
		fmt.Println(new(big.Float).Sub(inf, inf).String())
	}()
	fmt.Println(a.String(), third.String(), b.String(), big1.String(), third.Text('G', 4))
	fmt.Println(big.NewFloat(math.MaxFloat64).Text('g', 5), new(big.Float).SetInt(big.NewInt(-12345)).Text('f', 2))
	// Compute e with 100 bits.
	e := new(big.Float).SetPrec(100).SetInt64(1)
	term := new(big.Float).SetPrec(100).SetInt64(1)
	for k := int64(1); k < 40; k++ {
		term.Quo(term, new(big.Float).SetInt64(k))
		e.Add(e, term)
	}
	fmt.Println(e.Text('g', 30))
}
