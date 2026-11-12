package main

import (
	"io"
	"strings"
)

func isCloser(r io.Reader) bool {
	_, ok := r.(io.Closer)
	return ok
}

func main() {
	println(isCloser(strings.NewReader("test")))
}

// Output:
// false
