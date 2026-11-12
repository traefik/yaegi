package main

import "fmt"

var (
	m = l[0]
	n = l[1:3]
	o = len(l)
	l = "asdf"
)

func main() {
	fmt.Println(m, n, o)
}

// Output:
// 97 sd 4
