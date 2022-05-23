package main

import "fmt"

func main() {
	var m = map[string]interface{}{"a": "a"}
	a, _ := m["a"]
	b, ok := a.(string)
	fmt.Println("a:", a, ", b:", b, ", ok:", ok)
}

// Output:
// a: a , b: a , ok: true
