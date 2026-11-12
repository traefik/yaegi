// fmt verbs on scalar values (bool, every integer kind, string, []byte,
// rune) with widths, precisions and the flags + - # 0 and space, and
// the * width and precision arguments.
package main

import "fmt"

func main() {
	ints := []any{0, 42, -42, int8(-128), int16(1234), int32(-7), int64(-1 << 63), uint(7), uint8(255), uint16(65535), uint32(1 << 31), uint64(1<<64 - 1), uintptr(10)}
	verbs := []string{"%v", "%d", "%+d", "% d", "%5d", "%-5d|", "%05d", "%x", "%X", "%#x", "%#X", "%o", "%#o", "%O", "%b", "%#b", "%c", "%q", "%U", "%#U", "%8.3d", "%-08d|", "%+.0d", "%.0d", "% x", "%#v", "%T", "%e", "%s"}
	for _, v := range ints {
		for _, f := range verbs {
			fmt.Printf(f+" ", v)
		}
		fmt.Println()
	}
	for _, b := range []bool{true, false} {
		fmt.Printf("%v %t %5t|%-6t|%#v %T %d %s\n", b, b, b, b, b, b, b, b)
	}
	strs := []string{"", "go", "héllo", "a\"b\\c\n", "\xff\x00", "日本語テキスト", " ​"}
	for _, s := range strs {
		fmt.Printf("%s|%q|%+q|%#q|%x|% x|%X|% X|%#x|%10s|%-10s|%.2s|%.3q|%5.1s|%#v|%T|%d|%v|%010s|\n", s, s, s, s, s, s, s, s, s, s, s, s, s, s, s, s, s, s, s)
	}
	bs := []byte("hi\xff")
	fmt.Printf("%s %q %x %X % x %v %d %#v %T %08b\n", bs, bs, bs, bs, bs, bs, bs, bs, bs, bs)
	var nb []byte
	fmt.Printf("%s|%q|%x|%v|%#v\n", nb, nb, nb, nb, nb)
	arr := [3]byte{'a', 'b', 'c'}
	fmt.Printf("%s %x %v %q\n", arr, arr, arr, arr)
	runes := []rune{'a', 'é', '\n', 0x1F600, 0xd800, -1, 0x110000}
	for _, r := range runes {
		fmt.Printf("%c|%q|%+q|%#q|%U|%#U|%d|%x|%v|%5c|%-4q|\n", r, r, r, r, r, r, r, r, r, r, r)
	}
	fmt.Printf("[%*d] [%-*d] [%.*f] [%*.*f] [%*s]\n", 6, 42, 6, 42, 2, 3.14159, 8, 3, 2.5, -5, "ab")
	fmt.Printf("[%*d] [%.*d]\n", "x", 1, 2.0, 3)
	fmt.Printf("[%*d]\n", 10000000, 1)
	fmt.Printf("[%.*d]\n", -1, 5)
	fmt.Printf("%+q %#+q %# x %#08x %-#8x| %+#o\n", "é", "é", "ab", 255, 255, 8)
	fmt.Printf("%08.3d|%+08d|% 08d|%-+8d|% -8d|\n", 7, 7, 7, 7, 7)
	fmt.Printf("%x %X %#x %x\n", -255, int8(-1), -1, uint8(200))
	fmt.Printf("%100.90d\n", 1)
	fmt.Printf("%c%c%c\n", 72, 'i', 0x263A)
	type MyInt int
	type MyStr string
	type MyBool bool
	fmt.Printf("%v %d %x %#v %T | %v %q %#v %T | %v %#v %T\n", MyInt(5), MyInt(5), MyInt(255), MyInt(5), MyInt(5), MyStr("s"), MyStr("s"), MyStr("s"), MyStr("s"), MyBool(true), MyBool(true), MyBool(true))
	fmt.Printf("%%|%5%|%-5%|%c|%v\n", 'x', "end")
}
