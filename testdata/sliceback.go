package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

type P struct{ name string }

func main() {
	s := []string{"b", "c", "a"}
	sort.Strings(s)
	fmt.Println(s)

	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], 258)
	fmt.Println(b)

	buf := make([]byte, 5)
	n, _ := strings.NewReader("hello world").Read(buf)
	fmt.Println(n, string(buf))

	// Elements moved by native code keep their identity.
	p1, p2 := &P{"x"}, &P{"y"}
	ps := []*P{p2, p1}
	sort.Slice(ps, func(i, j int) bool { return ps[i].name < ps[j].name })
	p1.name = "x2"
	fmt.Println(ps[0].name, ps[0] == p1, ps[1] == p2)

	ys := []int{5, 2, 8, 1, 9, 3, 7, 4, 6, 0}
	sort.Slice(ys, func(i, j int) bool { return ys[i] > ys[j] })
	fmt.Println(ys, sort.SliceIsSorted(ys, func(i, j int) bool { return ys[i] > ys[j] }))

	xs := []int{3, 1, 2}
	sort.Sort(sort.IntSlice(xs))
	fmt.Println(xs, bytes.Equal([]byte("a"), []byte("a")))
}
