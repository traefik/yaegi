package main

import "fmt"

type T struct {
	F func()
}

func main() {
	foos := []T{}

	for i := 0; i < 3; i++ {
		a := i
		n := fmt.Sprintf("i=%d", i)
		println(n)
		foos = append(foos, T{func() { println(i, a, n) }})
	}
	foos[0].F()
	foos[1].F()
	foos[2].F()
}

// Output:
// 3 0
// 3 1
// 3 2
