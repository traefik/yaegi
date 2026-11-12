package main

import "fmt"

// Large local arrays: a zero array is a fresh slice, and an array of plain
// values is copied in one step (GOROOT test fixedbugs/issue13169.go). The
// copies must stay independent, and zero values must keep their kind
// (strings, structs, nested arrays).

type pt struct{ x, y int }

func deep(d int) byte {
	if d == 0 {
		return 0
	}
	var b [1024]byte
	b[d] = byte(d)
	r := deep(d - 1)
	if b[d] != byte(d) || b[d+1] != 0 {
		panic("frame array shared")
	}
	return r + b[d]
}

func main() {
	fmt.Println(deep(32))

	var a [512]int
	a[3] = 7
	b := a // copy
	b[3] = 8
	fmt.Println(a[3], b[3], len(b))

	p := new([300]byte)
	p[299] = 1
	q := *p
	p[299] = 2
	fmt.Println(p[299], q[299])

	var s [100]string
	fmt.Printf("%q %d\n", s[50], len(s[50]+"x"))
	s2 := s
	s2[1] = "x"
	fmt.Println(s[1] == "", s2[1])

	var ps [100]pt
	ps2 := ps
	ps2[5].x = 3
	fmt.Println(ps[5], ps2[5])

	var nested [10][20]int
	n2 := nested
	n2[1][2] = 5
	fmt.Println(nested[1][2], n2[1][2])

	var ptrs [64]*int
	v := 4
	ptrs[0] = &v
	ptrs2 := ptrs
	*ptrs2[0] = 9
	ptrs2[1] = &v
	fmt.Println(v, ptrs[1] == nil, ptrs2[1] != nil)

	var ifs [64]any
	ifs[0] = pt{1, 2}
	ifs2 := ifs
	ifs2[0] = "s"
	fmt.Println(ifs[0], ifs2[0])

	type bytes1k [1024]byte
	var bk bytes1k
	bk[1] = 1
	bk2 := bk
	bk2[1] = 2
	fmt.Println(bk[1], bk2[1])
}
