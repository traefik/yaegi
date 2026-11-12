package main

import "fmt"

type str string

type arr [3]int

var (
	a = &x
	b = a[1]
	c = y[0]
	d = z[2]
	x = arr{1, 2, 3}
	y = str("xyz")
	z = []string{"a", "b", "c"}
)

func main() {
	fmt.Println(b, c, d)
}

// Output:
// 2 120 c
