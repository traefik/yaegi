package main

import "fmt"

func main() {
	s := make([]any, 0)
	s = append(s, 1)
	for _, v := range s {
		fmt.Println(v)
	}
}

// Output:
// 1
