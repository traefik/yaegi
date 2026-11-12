package main

import "net/url"

func main() {
	u := url.URL
	_ = u
}

// Error:
// ../_test/upstream-1196.go:6:7: type url.URL is not an expression
