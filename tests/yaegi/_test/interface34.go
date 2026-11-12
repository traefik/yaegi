package main

import "fmt"

func main() {
	s := [2]interface{}{1: "test", 0: 2}
	fmt.Println(s[0], s[1])
}

// Output:
// 2 test
