package main

import (
	"fmt"
	"net/url"
)

func main() {
	body := url.Values{
		"Action": {"none"},
	}
	fmt.Println(body)
}

// Output:
// map[Action:[none]]
