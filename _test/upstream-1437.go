package main

import "fmt"

func main() {
	s := "abc"
	p := s[0]
	ms := myString("xyz")
	fmt.Println(p, ms[1])
}

type myString string

// Output:
// 97 121
