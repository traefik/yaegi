package main

import "fmt"

func main() {
	m := make(map[string]any)
	m["A"] = string("A")
	fmt.Println(m["A"])
}

// Output:
// A
