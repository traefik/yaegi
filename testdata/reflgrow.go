package main

import (
	"fmt"
	"reflect"
)

// reflect.Append, reflect.AppendSlice, Value.Grow and reflect.Copy copy
// struct and array elements by value, like the builtin append and copy:
// after the slice grows, the old and the new array do not share memory.
// Run with -aot and with -aot -opaque: the interpreted reflect works on the
// VM slices themselves.

type S struct{ a, b int }

func main() {
	a := make([]S, 1, 1)
	b := reflect.Append(reflect.ValueOf(a), reflect.ValueOf(S{})).Interface().([]S)
	a[0] = S{7, 7}
	fmt.Println(a[0], b[0])

	a2 := make([][2]int, 1, 1)
	b2 := reflect.AppendSlice(reflect.ValueOf(a2), reflect.ValueOf([][2]int{{1, 1}})).Interface().([][2]int)
	a2[0] = [2]int{6, 6}
	b2[1][0] = 5
	fmt.Println(a2, b2)

	c := make([]S, 1, 1)
	old := c
	reflect.ValueOf(&c).Elem().Grow(4)
	c[0] = S{8, 8}
	fmt.Println(old[0], c[0], cap(c) >= 5)

	// reflect.Copy writes into the memory of the destination
	src := []S{{1, 2}}
	dst := make([]S, 1)
	p := &dst[0]
	reflect.Copy(reflect.ValueOf(dst), reflect.ValueOf(src))
	dst[0].a = 9
	fmt.Println(src[0], dst[0], *p)

	// room left: Append writes into the array, the old slice sees it
	r := make([]S, 1, 2)
	q := reflect.Append(reflect.ValueOf(r), reflect.ValueOf(S{3, 3})).Interface().([]S)
	q[0].a = 4
	fmt.Println(r[0], q, r[:2][1])
}
