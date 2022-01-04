package main

import (
	"io"
	"os"
)

func f(i interface{}) {
	switch at := i.(type) {
	case int, int8:
		println("integer", at)
	case io.Reader:
		println("reader")
	}
	println("bye")
}

func main() {
	var fd *os.File
	var r io.Reader = fd
	f(r)
}

// Output:
// reader
// bye
