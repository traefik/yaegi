package main

import "fmt"

func main() {
	users := make(map[string]string)

	v := users["a"]
	fmt.Println("v:", v)
}

// Output:
// v:
