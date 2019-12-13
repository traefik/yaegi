package main

import "unicode/utf8"

func main() {
	r, _ := utf8.DecodeRuneInString("Hello")
	println(r < utf8.RuneSelf)
}

// Output:
// true
