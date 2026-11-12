package main

import "fmt"

const itoa64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func main() {
	fmt.Printf("%v %T\n", itoa64[2], itoa64[2])
}

// Output:
// 48 uint8
