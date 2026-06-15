package main

import "fmt"

func main() {
	s := make([]any, 0)
	s = append(s, 1)
	fmt.Println(s[0])
}

// Output:
// 1
