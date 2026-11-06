package main

import "fmt"

func f() string { return "a" }

func g() string { return "c" }

func main() {
	var x, y, z string

	// A call and a constant in the same assignment: both destinations must be set.
	x, y = f(), "b"
	fmt.Println(x, y)

	x, y = "b", f()
	fmt.Println(x, y)

	x, y, z = "p", f(), "q"
	fmt.Println(x, y, z)

	// All operands are evaluated before any destination is set.
	x, y = "old", "?"
	x, y = f(), x
	fmt.Println(x, y)

	x, y = y, g()
	fmt.Println(x, y)

	x, y = f(), g()
	fmt.Println(x, y)

	// The second call reads the destination of the first one: it must see the old value.
	x, y = "old", "?"
	h := func() string { return x }
	x, y = f(), h()
	fmt.Println(x, y)

	x = f()
	fmt.Println(x)
}

// Output:
// a b
// b a
// p a q
// a old
// old c
// a c
// a old
// a
