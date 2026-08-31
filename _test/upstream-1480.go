package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// buffer mimics the buffer type of golang/glog: a struct embedding a binary
// (stdlib) bytes.Buffer, whose promoted Write method must make *buffer an
// io.Writer. See glog.go l.print and yaegi issue #1480.
type buffer struct {
	bytes.Buffer
	tmp [64]byte
}

func clean(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "\n", " | ")
}

func main() {
	var buf buffer
	fmt.Fprintln(&buf, "hello")
	fmt.Println("fprintln:", clean(buf.String()))

	var w io.Writer = &buf
	fmt.Fprintf(w, "count %d", 42)
	fmt.Println("writer:", clean(buf.String()))

	io.WriteString(&buf, " done")
	fmt.Println("writeString:", clean(buf.String()))
}

// Output:
// fprintln: hello
// writer: hello | count 42
// writeString: hello | count 42 done
