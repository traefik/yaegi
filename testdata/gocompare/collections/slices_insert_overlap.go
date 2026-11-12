// slices.Insert and slices.Replace with values that overlap the slice:
// slices.overlaps compares the memory of the two slices (the unsafe
// addresses of the VM depend on the subslice, docs/UNSAFE.md).
package main

import (
	"fmt"
	"slices"
)

type pt struct{ x, y int }

func main() {
	b := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	fmt.Println(slices.Insert(b[:1], 0, b[1:3]...))
	c := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	fmt.Println(slices.Insert(c[:5], 2, c[4:8]...))
	s := []string{"a", "b", "c", "d", "e"}
	fmt.Println(slices.Replace(s[:3], 0, 1, s[1:4]...))
	ps := []pt{{1, 1}, {2, 2}, {3, 3}, {4, 4}}
	fmt.Println(slices.Insert(ps[:2], 1, ps[2:]...))
	bs := []byte("abcdefgh")
	fmt.Println(string(slices.Insert(bs[:2], 1, bs[2:5]...)))
	// no overlap: two arrays
	x, y := []int{1, 2, 3}, []int{7, 8}
	fmt.Println(slices.Insert(x[:1], 1, y...), x)
}
