package main

import "fmt"

func main() {
	a := int32(15) ^ rune(0)
	fmt.Printf("%T %v\n", a, a)
}

// Output:
// int32 15
