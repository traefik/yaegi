package main

import "fmt"

type T struct{ A, B string }

func main() {
	var t T
	var s string
	var sl []int
	var n int

	// A composite literal and a constant in the same assignment: both destinations must be set.
	t, s = T{"a", "b"}, "c"
	fmt.Println(t, s)

	s, t = "d", T{"e", "f"}
	fmt.Println(t, s)

	sl, n = []int{1, 2}, 3
	fmt.Println(sl, n)

	u, v := T{"g", "h"}, "i"
	fmt.Println(u, v)

	w, m := map[string]int{"k": 1}, 2
	fmt.Println(w, m)
}

// Output:
// {a b} c
// {e f} d
// [1 2] 3
// {g h} i
// map[k:1] 2
