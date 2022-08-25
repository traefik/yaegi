package main

import (
	"io"
	"log"
	"os"
	"strings"
)

type WrappedReader struct {
	reader io.Reader
}

func (wr WrappedReader) Read(p []byte) (n int, err error) {
	return wr.reader.Read(p)
}

// Of course, this implementation is completely stupid because it does not write
// to the intended writer, as any honest WriteTo implementation should. its
// implemtion is just to make obvious the divergence of behaviour with yaegi.
func (wr WrappedReader) WriteTo(w io.Writer) (n int64, err error) {
	// Ignore w, send to Stdout to prove whether this WriteTo is used.
	data, err := io.ReadAll(wr)
	if err != nil {
		return 0, err
	}
	nn, err := os.Stdout.Write(data)
	return int64(nn), err
}

func main() {
	f := strings.NewReader("hello world")
	wr := WrappedReader{reader: f}

	// behind the scenes, io.Copy is supposed to use wr.WriteTo if the implementation exists.
	// With Go, it works as expected, i.e. the output is sent to os.Stdout.
	// With Yaegi, it doesn't, i.e. the output is sent to io.Discard.
	if _, err := io.Copy(io.Discard, wr); err != nil {
		log.Fatal(err)
	}
}

// Output:
// hello world
