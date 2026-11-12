// fmt of float32 and float64 with the verbs e E f F g G x X b v and the
// flags + - # 0 space, widths, precisions, and the shortest form of %v.
package main

import (
	"fmt"
	"math"
)

func main() {
	vals := []float64{0, math.Copysign(0, -1), 1, -1, 0.5, 0.1, 1.0 / 3, 2.0 / 3, 123456.789, 1e20, 1e21, 1e-4, 1e-5,
		1.5e300, 5e-324, math.MaxFloat64, 100, 1234567, 12345678, 0.000001234, 2.5, 3.5, -0.125, 9.995, 0.045}
	verbs := []string{"%v", "%e", "%E", "%f", "%F", "%g", "%G", "%x", "%X", "%b", "%.0f", "%.1f", "%.2f", "%.3e", "%.10g",
		"%.3g", "%10.3f|", "%-10.3f|", "%010.3f", "%+.2e", "% .2f", "%#g", "%#.3g", "%#.0f", "%#e", "%#x", "%.2x", "%+v", "%12v|", "%.0e"}
	for _, v := range vals {
		for _, f := range verbs {
			fmt.Printf(f+" ", v)
		}
		fmt.Println()
		v32 := float32(v)
		for _, f := range verbs {
			fmt.Printf(f+" ", v32)
		}
		fmt.Println()
	}
	fmt.Println(float32(0.1), float64(float32(0.1)), float32(1)/3, 1e100, float32(1e-10), 1e6, 1e21-1, 123456789.0)
	fmt.Print(0.1, 0.2, " ", 1.5, "\n")
	fmt.Println(fmt.Sprint(3.0), fmt.Sprintf("%v", float32(16777216)), fmt.Sprintf("%6.2f%%", 99.5))
	fmt.Printf("%*.*f|%-*.*e|\n", 10, 2, math.Pi, 12, 3, -math.E)
	fmt.Printf("%v %.2f %g\n", []float64{0.5, 1e-9}, []float32{1.234, 5.678}, [2]float64{1e7, 1e-7})
	type Temp float64
	fmt.Printf("%v %.1f %#v %T\n", Temp(36.6), Temp(-40), Temp(2), Temp(0))
	fmt.Printf("%#v %#v %#v %#v\n", 1.0, 1e21, float32(0.5), []float64{1, 2.5})
	fmt.Printf("%d %s %q\n", 1.5, 2.5, 3.5)
	fmt.Printf("%.3v %10.4v| %-10.2v|\n", math.Pi, 123.456789, 1e-9)
	// Rounding of halves in %f: the decimal value of the binary float decides.
	for _, v := range []float64{0.5, 1.5, 2.5, 0.125, 0.375, 1.005, 2.675, 1.25, 1.35, -0.5, -2.5} {
		fmt.Printf("%.0f %.1f %.2f | ", v, v, v)
	}
	fmt.Println()
}
