package main

import "fmt"

type T [2]int

func F1(t *T) { t[0] = 1 }

func main() {
	t := &T{}
	F1(t)
	fmt.Println(t)
}

// Output:
// &[1 0]
