package main

import (
	"bytes"
	"io"
)

type TMemoryBuffer struct {
	*bytes.Buffer
	size int
}

func newTMemoryBuffer() *TMemoryBuffer {
	return &TMemoryBuffer{}
}

var globalMemoryBuffer = newTMemoryBuffer()

type TTransport interface {
	io.ReadWriter
}

func check(t TTransport) {
	println("ok")
}

func main() {
	check(globalMemoryBuffer)
}

// Output:
// ok
