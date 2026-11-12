// strconv.FormatInt, FormatUint, FormatFloat with every format byte, every
// bit size and many precisions, FormatComplex, FormatBool, Itoa, and the
// Append functions.
package main

import (
	"fmt"
	"math"
	"strconv"
)

func main() {
	for _, v := range []int64{0, 1, -1, 35, -36, 255, 1 << 40, math.MaxInt64, math.MinInt64} {
		for _, base := range []int{2, 3, 7, 8, 10, 16, 32, 36} {
			fmt.Print(strconv.FormatInt(v, base), " ")
		}
		fmt.Println()
	}
	for _, v := range []uint64{0, 9, 10, 1<<64 - 1, 1 << 63} {
		for _, base := range []int{2, 5, 10, 16, 36} {
			fmt.Print(strconv.FormatUint(v, base), " ")
		}
		fmt.Println()
	}
	fmt.Println(strconv.Itoa(-42), strconv.Itoa(0), strconv.FormatBool(true), strconv.FormatBool(false))
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		fmt.Println(strconv.FormatInt(5, 1))
	}()
	func() {
		defer func() { fmt.Println("recovered:", recover()) }()
		fmt.Println(strconv.FormatUint(5, 37))
	}()
	vals := []float64{0, math.Copysign(0, -1), 1, -1.5, 0.1, 1.0 / 3, 123456789.125, 1e21, 1e-7, 5e-324, math.MaxFloat64,
		math.Inf(1), math.Inf(-1), math.NaN(), 100, 0.000123, 2.5, 3.5, 1e6, 999999.5}
	for _, v := range vals {
		for _, f := range []byte{'b', 'e', 'E', 'f', 'g', 'G', 'x', 'X'} {
			for _, prec := range []int{-1, 0, 1, 3, 17} {
				fmt.Print(strconv.FormatFloat(v, f, prec, 64), " ")
			}
			fmt.Print(strconv.FormatFloat(v, f, -1, 32), " ")
			fmt.Print(strconv.FormatFloat(v, f, 2, 32), "|")
		}
		fmt.Println()
	}
	fmt.Println(strconv.FormatFloat(1, 'z', -1, 64), strconv.FormatFloat(1.5, 'f', 40, 64), strconv.FormatFloat(1e23, 'g', -1, 64))
	fmt.Println(strconv.FormatComplex(1+2i, 'f', 2, 128), strconv.FormatComplex(complex(math.Inf(1), -1), 'g', -1, 64), strconv.FormatComplex(-3i, 'e', 1, 128))
	b := []byte("n=")
	b = strconv.AppendInt(b, -12345, 10)
	b = append(b, ' ')
	b = strconv.AppendUint(b, 0xBEEF, 16)
	b = append(b, ' ')
	b = strconv.AppendFloat(b, 3.25, 'e', 3, 64)
	b = append(b, ' ')
	b = strconv.AppendFloat(b, 0.1, 'g', -1, 32)
	b = append(b, ' ')
	b = strconv.AppendBool(b, true)
	b = append(b, ' ')
	b = strconv.AppendQuote(b, "hé\n")
	b = append(b, ' ')
	b = strconv.AppendQuoteRune(b, '☺')
	b = append(b, ' ')
	b = strconv.AppendQuoteToASCII(b, "hé")
	b = append(b, ' ')
	b = strconv.AppendQuoteRuneToASCII(b, 'é')
	fmt.Println(string(b))
	// Round trips: shortest formatting parses back to the same value.
	ok := 0
	x := 1.0
	for i := 0; i < 500; i++ {
		x = float64(x*1.37) + 0.001
		if i%50 == 0 {
			x = float64(1 / x)
		}
		s := strconv.FormatFloat(x, 'g', -1, 64)
		y, _ := strconv.ParseFloat(s, 64)
		if y == x {
			ok++
		}
		s32 := strconv.FormatFloat(float64(float32(x)), 'g', -1, 32)
		y32, _ := strconv.ParseFloat(s32, 32)
		if float32(y32) == float32(x) {
			ok++
		}
	}
	fmt.Println(ok, strconv.FormatFloat(x, 'g', -1, 64))
}
