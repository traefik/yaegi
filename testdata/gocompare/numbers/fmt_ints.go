// fmt of integers of every type with the verbs d x X o O b c q U v and the
// flags + - # 0 space, widths and precisions, also given with *.
package main

import (
	"fmt"
	"math"
)

func main() {
	vals := []any{int8(-128), int8(127), uint8(255), int16(-1), uint16(65535), int32(math.MinInt32), uint32(math.MaxUint32),
		int64(math.MinInt64), uint64(math.MaxUint64), int(-42), uint(42), uintptr(0xBEEF), int64(0), int32(7)}
	verbs := []string{"%d", "%5d", "%-5d|", "%05d", "%+d", "% d", "%x", "%X", "%#x", "%#X", "%08x", "%o", "%#o", "%O", "%b",
		"%#b", "%.3d", "%8.3d", "%-+8d|", "%+08d", "% 08d", "%v", "%+v", "%#v", "%T", "%#08x", "%x.", "% x", "%010b", "%.0d", "%-#10o|"}
	for _, v := range vals {
		for _, f := range verbs {
			fmt.Printf(f+" ", v)
		}
		fmt.Println()
	}
	for _, r := range []rune{'A', 'é', '☺', 0x1F600, 0x10FFFF} {
		fmt.Printf("%c %q %U %#U %d %x %#q %5c|%-4q|\n", r, r, r, r, r, r, r, r, r)
	}
	fmt.Printf("%c %q %U\n", -1, rune(0xD800), 0x110000)
	fmt.Printf("%*d|%-*d|%.*d|%*.*d|\n", 6, 42, 6, 42, 4, 42, 8, 5, -42)
	fmt.Printf("%*d|\n", -6, 7)
	fmt.Printf("%.0d|%.0x|%5.0d|\n", 0, 0, 0)
	fmt.Printf("%d %d\n", []int{1, -2}, [2]uint8{3, 4})
	fmt.Printf("%x %X %#x\n", []int32{255, -255}, []uint16{0xABC}, []int{10})
	fmt.Printf("%08.3f|%x|%s|%d\n", 42, 3.5, 12, "str")
	fmt.Printf("%d %d\n", 1)
	fmt.Printf("%d\n", 1, 2)
	fmt.Printf("%!|%z|%-z\n", 3, 4)
	fmt.Printf("%[2]d %[1]d %[2]x %d\n", 10, 20)
	fmt.Printf("%[3]d|%[0]d\n", 1, 2)
	type MyInt int16
	type MyU uint32
	fmt.Printf("%v %d %x %#v %T\n", MyInt(-5), MyInt(-5), MyU(255), MyU(255), MyInt(1))
	fmt.Println(fmt.Sprint(int8(-1), uint8(1), 2, 3), fmt.Sprintln(int64(-1), uint64(1)))
	var pi *int
	fmt.Printf("%d %v\n", pi, pi)
	fmt.Printf("%v %d\n", []any{int8(1), uint64(2)}, map[string]int{"b": 2, "a": -1})
	fmt.Printf("%6.2v|%-6v|%06v\n", 12345, int8(-3), -7)
}
