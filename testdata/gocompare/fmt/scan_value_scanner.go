// fmt scanning with a fmt.Scanner of the program whose Scan method has a
// value receiver, given as a value (not a pointer).
package main

import (
	"fmt"
	"strings"
)

type sc struct{ p *string }

func (s sc) Scan(st fmt.ScanState, verb rune) error {
	tok, err := st.Token(true, nil)
	*s.p = "scanned:" + string(tok) + ":" + string(verb)
	return err
}

type bag map[string]int

func (b bag) Scan(st fmt.ScanState, verb rune) error {
	tok, err := st.Token(true, nil)
	b[string(tok)]++
	return err
}

type plain struct{ s string }

func (p plain) String() string { return p.s }

func main() {
	var got string
	var n int
	c, err := fmt.Sscan("a 7", sc{&got}, &n)
	fmt.Println(c, err, got, n)

	c, err = fmt.Sscanf("x-3", "%v-%d", sc{&got}, &n)
	fmt.Println(c, err, got, n)

	b := bag{}
	c, err = fmt.Fscan(strings.NewReader("k k j"), b, b, b)
	fmt.Println(c, err, b)

	p := sc{&got}
	c, err = fmt.Sscan("ptr", &p)
	fmt.Println(c, err, got)

	c, err = fmt.Sscan("no", plain{"v"})
	fmt.Println(c, err)
	c, err = fmt.Sscan("no", bare{1})
	fmt.Println(c, err)
	c, err = fmt.Sscan("1 2", &n, 3)
	fmt.Println(c, err, n)

	// a named pointer type is a pointer
	c, err = fmt.Sscan("5", pi(&n))
	fmt.Println(c, err, n)
	var bs []byte
	c, err = fmt.Sscan("hey", pb(&bs))
	fmt.Println(c, err, string(bs))
	var a any = pi(&n)
	c, err = fmt.Sscanf("8", "%d", a)
	fmt.Println(c, err, n)
}

type pi *int

type pb *[]byte

type bare struct{ n int }
