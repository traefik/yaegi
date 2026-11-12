package main

import "fmt"

// The print and println builtins format their arguments like the gc
// runtime: nil pointers, maps, chans and funcs are 0x0, a nil slice is
// [0/0]0x0, a nil interface is (0x0,0x0), and floats use the shortest form
// for their size. Integer and []rune conversions to string give "�"
// for invalid code points.

type stringer interface{ String() string }

func main() {
	println((*int)(nil), (chan int)(nil), (map[string]int)(nil), (func())(nil))
	println([]int(nil), []byte(nil))
	println(any(nil), stringer(nil))
	println(1.5, float32(0.1), 1e21, complex64(complex(0.1, 2)), complex(9.0, 10.0))
	print("a", 1, true, "\n")
	defer println((*int)(nil), []string(nil), any(nil), float32(0.1))

	var big int64 = 1<<32 + 'A'
	var neg int32 = -1
	var u uint64 = 1 << 63
	fmt.Printf("%q %q %q %q\n", string(rune(big)), string(big), string(neg), string(u))
	rs := []rune("héllo, 世界\xff")
	fmt.Println(len(rs), rs)
	fmt.Printf("%q\n", string(rs))
	fmt.Printf("%q\n", string([]rune{0x10ffff, 0x10ffff + 1, 0xD800, 0xDFFF, -1}))
}
