package main

import "fmt"

type T struct{ s string }

func main() {
	ch := make(chan string, 6)
	for _, v := range []string{"a", "b", "c", "d", "e", "f"} {
		ch <- v
	}

	// A variable of an enclosing function.
	x := "old"
	func() { x = <-ch }()
	fmt.Println(x)

	// A struct field and a slice element.
	var t T
	t.s = <-ch
	a := []string{"p", "q"}
	a[1] = <-ch
	fmt.Println(t.s, a)

	// A pointer taken before the receive sees the new value.
	var y string
	p := &y
	y = <-ch
	fmt.Println(*p)

	// So does a closure created before the receive.
	var z string
	read := func() string { return z }
	z = <-ch
	fmt.Println(read())

	// A dereferenced pointer.
	var w string
	pw := &w
	*pw = <-ch
	fmt.Println(w)
}

// Output:
// a
// b [p c]
// d
// e
// f
