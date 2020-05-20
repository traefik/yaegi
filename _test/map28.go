package main

import (
	"net/url"
)

func main() {
	value1 := url.Values{}

	value1.Set("first", "v1")
	value1.Set("second", "v2")

	l := 0
	for k, v := range value1 {
		l += len(k) + len(v)
	}
	println(l)
}

// Output:
// 13
