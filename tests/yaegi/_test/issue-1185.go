package main

import "io"

type B []byte

func (b B) Write(p []byte) (n int, err error) {
	b = p
	return len(p), nil
}

func main() {
	b := B{}
	a := make([]io.Writer, 0)
	a = append(a, b)
	println(len(a))
}

// Output:
// 1
