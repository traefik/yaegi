package main

import "fmt"

// == on interfaces that hold values that cannot be compared panics, and
// struct and array comparisons happen in the order of gc: the program sees
// which comparison panics (GOROOT test/fixedbugs/issue8606.go).

type S struct {
	x, y any
}

type T struct {
	i any
	a int64
	j any
}

type U struct {
	s string
	n int
}

func try(name string, f func() bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(name, "panic:", r)
		}
	}()
	fmt.Println(name, f())
}

func main() {
	b := []byte{1}
	var sl []int
	var m map[int]int
	var fn func()
	try("slice", func() bool { var x any = sl; return x == x })
	try("map", func() bool { var x any = m; return x == x })
	try("func", func() bool { var x any = fn; return x == x })
	try("struct field", func() bool { var x any = struct{ f []int }{}; return x == x })
	try("int", func() bool { var x any = 1; return x == x })
	try("map key", func() bool { k := make(map[any]int); k[sl] = 1; return true })

	try("S first differs", func() bool { return any(S{1, b}) == any(S{2, b}) })
	try("S second differs", func() bool { return any(S{b, 1}) == any(S{b, 2}) })
	try("T", func() bool { return any(T{1, 1, b}) == any(T{1, 2, b}) })
	try("T panics", func() bool { return any(T{b, 1, 1}) == any(T{b, 2, 1}) })
	try("array", func() bool { return any([2]any{1, b}) == any([2]any{2, b}) })
	try("array panics", func() bool { return any([2]any{b, 1}) == any([2]any{b, "2"}) })
	try("strings", func() bool { return U{"ab", 1} == U{"ab", 1} && [2]string{"a", "b"} != [2]string{"a", "c"} })
}
