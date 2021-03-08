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
		foos = append(foos, T{func() { println(i, a, n) }})
	}
	foos[0].F()
	foos[1].F()
	foos[2].F()
}

// Output:
// 3 0 i=0
// 3 1 i=1
// 3 2 i=2
