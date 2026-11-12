// testing/quick gets the funcs of the program as funcs: Check and
// CheckEqual look at them with reflect and call them (with -aot, a func in
// an any was an opaque handle: "argument is not a function").
package main

import (
	"fmt"
	"testing/quick"
)

type pair struct{ A, B int8 }

func main() {
	calls := 0
	err := quick.Check(func(a, b int) bool { calls++; return a+b == b+a }, nil)
	fmt.Println("Check:", err, calls)

	err = quick.Check(func(p pair) bool { return p.A+p.B == p.B+p.A }, &quick.Config{MaxCount: 5})
	fmt.Println("Check struct:", err)

	double := func(x uint8) int { return int(x) * 2 }
	add := func(x uint8) int { return int(x) + int(x) }
	fmt.Println("CheckEqual:", quick.CheckEqual(double, add, nil))

	err = quick.Check(func(x int) bool { return false }, nil)
	if ce, ok := err.(*quick.CheckError); ok {
		fmt.Println("failing Check:", ce.Count, len(ce.In))
	}

	err = quick.CheckEqual(func(x int) int { return x }, func(x int) int { return x + 1 }, nil)
	_, ok := err.(*quick.CheckEqualError)
	fmt.Println("failing CheckEqual:", ok)

	fmt.Println("not a func:", quick.Check(3, nil))
}
