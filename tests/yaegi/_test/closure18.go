package main

import "fmt"

type T struct {
	F func()
}

func main() {
	foos := []T{}

	for i := range 3 {
		a := i
		n := fmt.Sprintf("i=%d", i)
		foos = append(foos, T{func() { println(i, a, n) }})
	}
	foos[0].F()
	foos[1].F()
	foos[2].F()
}

// Output:
// 0 0 i=0
// 1 1 i=1
// 2 2 i=2
