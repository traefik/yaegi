// make with bad sizes from variables (negative len, cap below len, huge
// sizes), and conversions from slices to arrays and to array pointers,
// with the length panics recovered and printed as in Go.
package main

import "fmt"

func try(name string, f func()) {
	defer func() {
		r := recover()
		if err, ok := r.(error); ok {
			fmt.Println(name, "error:", err.Error())
			return
		}
		fmt.Println(name, "value:", r)
	}()
	f()
}

func main() {
	neg, two, one := -1, 2, 1
	try("make len neg", func() { _ = make([]int, neg) })
	try("make cap neg", func() { _ = make([]int, 0, neg) })
	try("make cap<len", func() { _ = make([]int, two, one) })
	try("make map neg", func() { m := make(map[int]int, neg); fmt.Println(len(m)) })
	try("make chan neg", func() { _ = make(chan int, neg) })
	var hugeLen int64 = 1 << 62
	try("make huge", func() { _ = make([]byte, hugeLen) })
	try("make huge cap", func() { _ = make([]int64, 0, hugeLen) })
	try("make ok", func() {
		s := make([]int, two, two+one)
		fmt.Println(len(s), cap(s))
	})

	// Slice to array: a copy.
	s := []int{1, 2, 3, 4}
	a := [4]int(s)
	a[0] = 100
	fmt.Println(s, a)
	a2 := [2]int(s[1:])
	fmt.Println(a2)
	a0 := [0]int(s)
	fmt.Println(a0, len(a0))
	var nilS []int
	fmt.Println([0]int(nilS))
	try("[5]int(s)", func() { _ = [5]int(s) })
	try("[1]int(nil)", func() { _ = [1]int(nilS) })

	// Slice to array pointer: shares the backing array.
	p := (*[3]int)(s)
	p[1] = 200
	fmt.Println(s, *p, len(p))
	p2 := (*[2]int)(s[2:])
	p2[0] = 300
	fmt.Println(s, *p2)
	fmt.Println((*[0]int)(nilS) == nil, (*[0]int)(s) != nil)
	try("(*[5]int)(s)", func() { _ = (*[5]int)(s) })
	try("(*[1]int)(nil)", func() { _ = (*[1]int)(nilS) })

	// Within capacity but not length still panics.
	c := make([]byte, 2, 8)
	try("(*[4]byte)(c)", func() { _ = (*[4]byte)(c) })
	try("[3]byte(c)", func() { _ = [3]byte(c) })
	fmt.Println([2]byte(c), *(*[8]byte)(c[:8]))

	// Named types.
	type Vec []float64
	type Arr [2]float64
	v := Vec{1.5, 2.5}
	arr := Arr(v)
	fmt.Printf("%v %T\n", arr, arr)
	pv := (*Arr)(v)
	pv[0] = 9
	fmt.Println(v)

	// Strings and bytes.
	b := []byte("abc")
	ba := [3]byte(b)
	fmt.Println(string(ba[:]), ba)
	str := string(b[:two])
	fmt.Println(str)
	r := []rune("héllo")
	ra := [5]rune(r)
	fmt.Println(string(ra[:]), len(ra))
}
