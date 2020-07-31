package main

import "fmt"

func main() {
	t := make([]byte, 2)
	t[0] = '$'
	fmt.Println(t)
}

// Output:
// [36 0]
