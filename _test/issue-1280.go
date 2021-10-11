package main

import (
	"io"
	"log"
	"os"
)

type DBReader interface {
	io.ReadCloser
	io.ReaderAt
}

type DB struct {
	f DBReader
}

func main() {
	f, err := os.Open("/dev/null")
	if err != nil {
		log.Fatal(err)
	}
	d := &DB{f}
	data := make([]byte, 1)
	_, _ = d.f.ReadAt(data, 0)
	println("bye")
}

// Output:
// bye
