package main

import "fmt"

func main() {
	a := map[string]int{"hello": 1, "world": 3}
	delete(a, "hello")
	fmt.Println(a)
}

// Output:
// map[world:3]
