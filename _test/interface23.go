package main

import "fmt"

func main() {
	m := make(map[string]interface{})
	m["A"] = string("A")
	fmt.Println(m["A"])
}

// Output:
// A
