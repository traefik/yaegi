package main

import (
	"fmt"
	"io"
	"log"
	"strings"
)

type readAutoCloser struct {
	r io.ReadCloser
}

func (a readAutoCloser) Read(b []byte) (n int, err error) {
	if a.r == nil {
		return 0, io.EOF
	}
	n, err = a.r.Read(b)
	if err == io.EOF {
		a.Close()
	}
	return n, err
}

func (a readAutoCloser) Close() error {
	if a.r == nil {
		return nil
	}
	return a.r.(io.Closer).Close()
}

type pipe struct {
	Reader readAutoCloser
}

func newReadAutoCloser(r io.Reader) readAutoCloser {
	if _, ok := r.(io.Closer); !ok {
		return readAutoCloser{io.NopCloser(r)}
	}
	return readAutoCloser{r.(io.ReadCloser)}
}

func main() {
	p := &pipe{}
	p.Reader = newReadAutoCloser(strings.NewReader("test"))
	b, err := io.ReadAll(p.Reader)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(b))
}

// Output:
// test
