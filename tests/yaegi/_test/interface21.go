package main

import "fmt"

func main() {
	s := make([]interface{}, 1)
	s[0] = 1
	fmt.Println(s[0])
}

// Output:
// 1
