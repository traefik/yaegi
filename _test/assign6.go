package main

import "fmt"

func main() {
	t := map[string]int{"a": 1, "b": 2}
	fmt.Println(t)
	t["a"], t["b"] = t["b"], t["a"]
	fmt.Println(t)
}

// Output:
// map[a:1 b:2]
// map[a:2 b:1]
