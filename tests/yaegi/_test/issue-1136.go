package main

import (
	"fmt"
	"io"
)

type T struct {
	r io.Reader
}

func (t *T) Read(p []byte) (n int, err error) { n, err = t.r.Read(p); return }

func main() {
	x := io.LimitedReader{}
	y := io.Reader(&x)
	y = &T{y}
	fmt.Println(y.Read([]byte("")))
}

// Output:
// 0 EOF
