package main

import (
	"errors"
	"fmt"
)

var calls int

func closeIt() error {
	calls++
	return errors.New("x")
}

func main() {
	n := 0
	for _, s := range []string{"a", "bc"} {
		_ = closeIt()
		n += len(s)
	}
	fmt.Println(n, calls)

	m := 0
	for i, _ := range []int{5, 6, 7} {
		_ = closeIt()
		_, _ = "a", closeIt()
		m += i
	}
	fmt.Println(m, calls)
}

// Output:
// 3 2
// 3 8
