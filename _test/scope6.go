package main

import "fmt"

var a = [3]int{1, 2, 3}

func f() { fmt.Println(a) }

func main() {
	fmt.Println(a)
	a[1] = 5
	f()
}

// Output:
// [1 2 3]
// [1 5 3]
