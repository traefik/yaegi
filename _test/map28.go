package main

import (
	"fmt"
	"net/url"
)

func main() {
	value1 := url.Values{}

	value1.Set("first", "v1")
	value1.Set("second", "v2")

	for k, v := range value1 {
		fmt.Println("k:", k, "v:", v)
	}
}

// Output:
// k: first v: [v1]
// k: second v: [v2]
