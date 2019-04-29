package main

import "fmt"

func main() {
	t := map[string]int{"a": 1, "b": 2}
	fmt.Println(t["a"], t["b"])
	t["a"], t["b"] = t["b"], t["a"]
	fmt.Println(t["a"], t["b"])
}

// Output:
// 1 2
// 2 1
