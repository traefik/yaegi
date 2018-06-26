package main

import "fmt"

func main() {
	a := [6]int{1, 2, 3, 4, 5, 6}
	fmt.Println(a[2:])
}

// Output:
// [3 4 5 6]
