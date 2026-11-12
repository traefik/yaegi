package main

import "fmt"

func main() {
	s := [2]interface{}{"test", 2}
	fmt.Println(s[0], s[1])
}

// Output:
// test 2
