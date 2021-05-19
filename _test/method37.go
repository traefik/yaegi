package main

import (
	"net"
	"os"
)

func writeBufs(bufs ...[]byte) error {
	b := net.Buffers(bufs)
	_, err := b.WriteTo(os.Stdout)
	return err
}

func main() {
	writeBufs([]byte("hello"))
}

// Output:
// hello
