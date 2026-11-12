// fmt scanning into a nil pointer panics with a nil pointer dereference.
package main

import (
	"fmt"
	"strings"
)

func try(name string, f func() (int, error)) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(name, "recovered:", r)
		}
	}()
	n, err := f()
	fmt.Println(name, "no panic", n, err)
}

func main() {
	var p *int
	try("a", func() (int, error) { return fmt.Sscan("5", p) })
	var s *string
	try("b", func() (int, error) { return fmt.Sscanf("x", "%s", s) })
	var f *float64
	try("c", func() (int, error) { return fmt.Sscanln("1.5", f) })
	var b *bool
	try("d", func() (int, error) { return fmt.Fscan(strings.NewReader("true"), b) })
	var u *uint8
	try("e", func() (int, error) { return fmt.Sscan("7", u) })
	var q int
	try("f", func() (int, error) { return fmt.Sscan("5", &q) })
	fmt.Println("q", q)
	try("g", func() (int, error) { return fmt.Sscan("5", q) })
	var c *color
	try("h", func() (int, error) { return fmt.Sscan("5", c) })
	var bs *[]byte
	try("i", func() (int, error) { return fmt.Sscan("ab", bs) })
	fmt.Printf("j %v %T %v %T\n", p, p, bs, bs)
	var x any = p
	fmt.Println("k", x == nil, x.(*int) == nil)
}

type color int
