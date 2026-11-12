package main

import "fmt"

const a1 = 0x7f8 >> 3

func main() {
	fmt.Printf("%T %v\n", a1, a1)
}

// Output:
// int 255
