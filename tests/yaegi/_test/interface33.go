package main

import "fmt"

func main() {
	var a = map[string]interface{}{"test": "test"}
	fmt.Println(a["test"])
}

// Output:
// test
