package main

import (
	"fmt"
	"io"
)

type T []byte

func (t *T) Write(p []byte) (n int, err error) { *t = append(*t, p...); return len(p), nil }

func foo(w io.Writer) {
	a := w.(*T)
	fmt.Fprint(a, "test")
	fmt.Printf("%s\n", *a)
}

func main() {
	x := T{}
	foo(&x)
}

// Output:
// test
