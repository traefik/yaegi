// fmt verbs on float32, float64 and complex values: %v %e %E %f %F %g %G
// %b %x %X with widths, precisions, flags, NaN and the infinities, and
// strconv.FormatFloat for the same values.
package main

import (
	"fmt"
	"math"
	"strconv"
)

func main() {
	fs := []float64{0, math.Copysign(0, -1), 1, -1, 0.1, 1.0 / 3, 2.5, 123456789, 1e21, 1e20, 1e-7, 1e-5, 5e-324, math.MaxFloat64, math.SmallestNonzeroFloat64, math.Inf(1), math.Inf(-1), math.NaN(), 100, 0.000123, 1.5e300}
	verbs := []string{"%v", "%e", "%E", "%f", "%F", "%g", "%G", "%b", "%x", "%X", "%.3e", "%.2f", "%.0f", "%#.0f", "%.3g", "%#g", "%#.3g", "%10.2f|", "%-10.2f|", "%+.1f", "% .1f", "%010.3f", "%+010.1e", "%#x", "%.2x", "%8.3v|", "%#v", "%T", "%d", "%s"}
	for _, f := range fs {
		for _, v := range verbs {
			fmt.Printf(v+" ", f)
		}
		fmt.Println()
		for _, fmtc := range []byte{'e', 'f', 'g', 'b', 'x'} {
			fmt.Print(strconv.FormatFloat(f, fmtc, -1, 64), " ", strconv.FormatFloat(f, fmtc, 3, 64), " ", strconv.FormatFloat(f, fmtc, -1, 32), " ")
		}
		fmt.Println()
	}
	f32s := []float32{0.1, 1.0 / 3, 16777217, float32(math.Inf(1)), 3.4e38, 1e-45, -2.5}
	for _, f := range f32s {
		fmt.Printf("%v %g %e %.10f %x %#v %T %b\n", f, f, f, f, f, f, f, f)
	}
	cs := []complex128{0, 1 + 2i, complex(-1.5, -0.25), complex(math.Inf(1), math.NaN()), complex(1e21, 1e-7)}
	for _, c := range cs {
		fmt.Printf("%v %.2f %+.1e %g %#v %T %8.1f| %x\n", c, c, c, c, c, c, c, c)
	}
	var c64 complex64 = complex(1.5, -2)
	fmt.Printf("%v %T %.3g %#v\n", c64, c64, c64, c64)
	fmt.Println(1e6, 1e21, 1e-4, 1e-5, float32(0.1), 100.0, -0.0, 1.0, math.Copysign(0, -1), complex(1, 0))
	fmt.Println(fmt.Sprint(3.0), fmt.Sprint(float32(3)), fmt.Sprint(1e100), fmt.Sprint(123456789.0), fmt.Sprint(1234567890123456789.0))
	type Temp float64
	fmt.Printf("%v %.1f %#v %T\n", Temp(21.5), Temp(21.5), Temp(21.5), Temp(21.5))
	fmt.Printf("%6.2f%%\n", 99.5)
	fmt.Printf("%.60f\n", 0.1)
	fmt.Printf("%.30e\n", 1.0/3)
	fmt.Printf("%.1000f|\n", 1.0)
}
