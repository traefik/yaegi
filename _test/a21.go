package main

import "fmt"

func main() {
	a := []byte("hello")
	fmt.Println(a)
	a = append(a, '=')
	fmt.Println(a)
}

// Output:
// [104 101 108 108 111]
// [104 101 108 108 111 61]
