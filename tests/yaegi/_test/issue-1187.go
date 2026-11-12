package main

import (
	"io"
	"os"
)

type sink interface {
	io.Writer
	io.Closer
}

func newSink() sink {
	// return os.Stdout	// Stdout is special in yaegi tests
	file, err := os.CreateTemp("", "yaegi-test.*")
	if err != nil {
		panic(err)
	}
	return file
}

func main() {
	s := newSink()
	n, err := s.Write([]byte("Hello\n"))
	if err != nil {
		panic(err)
	}
	var writer io.Writer = s
	m, err := writer.Write([]byte("Hello\n"))
	if err != nil {
		panic(err)
	}
	var closer io.Closer = s
	err = closer.Close()
	if err != nil {
		panic(err)
	}
	err = os.Remove(s.(*os.File).Name())
	if err != nil {
		panic(err)
	}
	println(m, n)
}

// Output:
// 6 6
