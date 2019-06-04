package main

import "fmt"

func main() {
	a := [1]*int{}
	a[0] = new(int)
	*a[0] = 2
	fmt.Println(*a[0])
}

// Output:
// 2
