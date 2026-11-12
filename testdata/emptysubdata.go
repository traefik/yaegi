// The data word of an empty string or slice cut from another one is the
// data of that one, as in gc (GOROOT test slicecap.go): gc does not move
// the pointer past the end. A Go string of length 0 loses its data pointer
// in an interface, and in the VM every value is in one (Value.R). The data
// of a slice of cells is its address in the array the program knows (base
// plus the gc offset).
package main

import (
	"fmt"
	"unsafe"
)

var (
	word  = "abcdefgh"
	nums  = []int64{1, 2, 3, 4, 5, 6}
	small = []int16{1, 2, 3}
	n     = 8
)

func strData(s string) uintptr { return *(*uintptr)(unsafe.Pointer(&s)) }

func i64Data(s []int64) uintptr { return *(*uintptr)(unsafe.Pointer(&s)) }

func i16Data(s []int16) uintptr { return *(*uintptr)(unsafe.Pointer(&s)) }

type strHeader struct {
	data unsafe.Pointer
	len  int
}

func main() {
	base := strData(word)
	for _, c := range []struct {
		name string
		s    string
	}{
		{"word[2:]", word[2:]},
		{"word[8:]", word[8:]},
		{"word[n:]", word[n:]},
		{"word[3:3]", word[3:3]},
		{"word[1:][2:][5:]", word[1:][2:][5:]},
		{"word[6:][2:]", word[6:][2:]},
		{"word[4:][1:][3:]", word[4:][1:][3:]},
	} {
		fmt.Println(c.name, len(c.s), strData(c.s)-base)
	}

	// kept in a variable, a parameter, a field, and a header struct
	e := word[5:][3:]
	t := struct{ s string }{e}
	h := (*strHeader)(unsafe.Pointer(&t.s))
	fmt.Println(strData(e)-base, strData(t.s)-base, uintptr(h.data)-base, h.len)
	// an empty string put in an interface and back: as in gc, it may
	// lose its data, so only the length is printed
	var i any = e
	fmt.Println(len(i.(string)), e == "", i == "")

	nb := i64Data(nums)
	for _, c := range []struct {
		name string
		s    []int64
	}{
		{"nums[1:]", nums[1:]},
		{"nums[6:]", nums[6:]},
		{"nums[2:][4:]", nums[2:][4:]},
		{"nums[1:3:3][2:]", nums[1:3:3][2:]},
		{"nums[3:][1:]", nums[3:][1:]},
	} {
		fmt.Println(c.name, len(c.s), cap(c.s), i64Data(c.s)-nb)
	}

	sb := i16Data(small)
	fmt.Println(i16Data(small[1:])-sb, i16Data(small[1:][2:])-sb, i16Data(small[3:])-sb)
}
