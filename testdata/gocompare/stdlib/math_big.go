// math/big, math/bits, math/cmplx and strconv with program types: big.Int,
// big.Rat and big.Float wrapped in program types with String and Format
// methods, factorials and fractions, bits operations, and strconv parse
// errors printed. (fmt verbs on big.Int and big.Float, which use their
// Format method, are left out: they differ in yaegi -aot today.)
package main

import (
	"fmt"
	"math"
	"math/big"
	"math/bits"
	"math/cmplx"
	"strconv"
)

type Fact struct{ N int64 }

func (f Fact) Value() *big.Int { return new(big.Int).MulRange(1, f.N) }
func (f Fact) String() string  { return fmt.Sprintf("%d! = %s", f.N, f.Value()) }

type Frac struct{ r *big.Rat }

func NewFrac(a, b int64) Frac  { return Frac{big.NewRat(a, b)} }
func (f Frac) Add(o Frac) Frac { return Frac{new(big.Rat).Add(f.r, o.r)} }
func (f Frac) String() string  { return f.r.String() }

func main() {
	for _, n := range []int64{0, 1, 5, 20, 25, 40} {
		fmt.Println(Fact{n}, Fact{n}.Value().BitLen())
	}
	a, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
	b, ok := new(big.Int).SetString("-ff", 16)
	fmt.Println(a, b, ok)
	q, r := new(big.Int).QuoRem(a, big.NewInt(97), new(big.Int))
	d, m := new(big.Int).DivMod(big.NewInt(-7), big.NewInt(3), new(big.Int))
	fmt.Println(q, r, d, m, new(big.Int).Quo(big.NewInt(-7), big.NewInt(3)), new(big.Int).Rem(big.NewInt(-7), big.NewInt(3)))
	fmt.Println(new(big.Int).Exp(big.NewInt(2), big.NewInt(100), nil), new(big.Int).Exp(big.NewInt(3), big.NewInt(200), big.NewInt(1000007)))
	fmt.Println(new(big.Int).GCD(nil, nil, big.NewInt(462), big.NewInt(1071)), big.NewInt(97).ProbablyPrime(10), new(big.Int).Sqrt(a), a.Cmp(b), b.Sign(), new(big.Int).Abs(b))
	fmt.Println(a.Text(16), b.Text(16), big.NewInt(64).Text(8), big.NewInt(5).Text(2), b.String())
	fmt.Println(a.Text(36), a.Int64(), a.IsInt64(), b.Int64(), new(big.Int).Lsh(big.NewInt(1), 70), new(big.Int).Rsh(a, 64), new(big.Int).And(a, big.NewInt(0xffff)), new(big.Int).Not(big.NewInt(5)))
	_, ok = new(big.Int).SetString("12x", 10)
	fmt.Println(ok)
	tb, _ := a.MarshalText()
	var a2 big.Int
	fmt.Println(string(tb), a2.UnmarshalText([]byte("bad")), a2.UnmarshalText(tb), a2.Cmp(a))
	sum := NewFrac(0, 1)
	for i := int64(1); i <= 10; i++ {
		sum = sum.Add(NewFrac(1, i*i))
	}
	fmt.Println(sum, sum.r.FloatString(10), sum.r.Num(), sum.r.Denom(), NewFrac(6, -4), NewFrac(0, 5))
	rt, ok := new(big.Rat).SetString("3.125")
	rt2, ok2 := new(big.Rat).SetString("1/0")
	fmt.Println(rt, ok, rt2, ok2, rt.IsInt(), new(big.Rat).Inv(rt), new(big.Rat).Mul(rt, rt).FloatString(3))
	f, _ := rt.Float64()
	fmt.Println(f)
	x := new(big.Float).SetPrec(200)
	x.SetString("1.5")
	y := new(big.Float).SetPrec(200).Quo(big.NewFloat(1), big.NewFloat(3))
	z := new(big.Float).SetPrec(200).Mul(x, y)
	fmt.Println(x, y.Text('g', 30), z.Text('f', 40), z.Prec(), y.MinPrec(), x.Cmp(y), new(big.Float).Sqrt(big.NewFloat(2)).Text('g', 20))
	fmt.Println(y.Text('f', 10), big.NewFloat(1e100).Text('g', -1), big.NewFloat(-2.5).Text('e', 6), new(big.Float).SetInf(true))
	fl, acc := y.Float64()
	i64, acc2 := big.NewFloat(1e30).Int64()
	fmt.Println(fl, acc, i64, acc2)
	pf, _, err := big.ParseFloat("0x1p-3", 0, 53, big.ToNearestEven)
	fmt.Println(pf, err)
	_, _, err = big.ParseFloat("1.2.3", 10, 53, big.ToNearestEven)
	fmt.Println(err)

	// math/bits.
	for _, u := range []uint64{0, 1, 0x80, 0xf0f0, 1 << 63, math.MaxUint64} {
		fmt.Println(u, bits.LeadingZeros64(u), bits.TrailingZeros64(u), bits.OnesCount64(u), bits.Len64(u), bits.Reverse64(u), bits.ReverseBytes64(u), bits.RotateLeft64(u, 3), bits.RotateLeft64(u, -3))
	}
	hi, lo := bits.Mul64(math.MaxUint64, 3)
	s, c := bits.Add64(math.MaxUint64, 1, 0)
	df, bo := bits.Sub64(0, 1, 0)
	qq, rr := bits.Div64(1, 0, 3)
	fmt.Println(hi, lo, s, c, df, bo, qq, rr, bits.Len8(7), bits.OnesCount16(0xffff), bits.Reverse8(1), bits.RotateLeft32(1, -1))
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		bits.Div64(5, 0, 3)
	}()

	// math/cmplx.
	for _, z := range []complex128{0, 1i, -1, complex(3, 4), complex(math.Inf(1), 0)} {
		fmt.Printf("%v abs=%.4f phase=%.4f sqrt=%.4f exp=%.4f conj=%v inf=%v nan=%v\n", z, cmplx.Abs(z), cmplx.Phase(z), cmplx.Sqrt(z), cmplx.Exp(z), cmplx.Conj(z), cmplx.IsInf(z), cmplx.IsNaN(z))
	}
	rr2, th := cmplx.Polar(complex(0, 2))
	fmt.Printf("%.4f %.4f %.4f\n", rr2, th, cmplx.Rect(2, math.Pi/2))

	// strconv errors.
	for _, s := range []string{"12", "-0", "0x1f", "1_000", "9223372036854775808", "abc", "", "+5", "1e3"} {
		i, err := strconv.ParseInt(s, 0, 64)
		u8, err2 := strconv.ParseUint(s, 10, 8)
		f, err3 := strconv.ParseFloat(s, 64)
		fmt.Println(s, i, err, u8, err2, f, err3)
	}
	for _, s := range []string{"true", "T", "0", "yes"} {
		bv, err := strconv.ParseBool(s)
		fmt.Println(bv, err)
	}
	fmt.Println(strconv.Quote("tab\there é \x00 ☺"), strconv.QuoteToASCII("é☺"), strconv.QuoteRune('\''), strconv.Itoa(-45), strconv.FormatInt(-255, 16), strconv.FormatUint(255, 2))
	uq, err := strconv.Unquote(`"a\tbé"`)
	_, err2 := strconv.Unquote(`'ab'`)
	fmt.Printf("%q %v %v\n", uq, err, err2)
	fmt.Println(strconv.FormatFloat(1.0/3, 'e', -1, 32), strconv.FormatFloat(123456789, 'g', 4, 64), strconv.FormatFloat(-0.0, 'f', 2, 64), strconv.AppendInt([]byte("n="), 42, 10), strconv.FormatComplex(complex(1, -2), 'f', 1, 128))
}
