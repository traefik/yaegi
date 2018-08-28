package main

import (
	"crypto/rand"
	"fmt"
	"io"
)

func main() {
	var buf [16]byte
	fmt.Println(buf)
	io.ReadFull(rand.Reader, buf[:])
	//io.ReadFull(rand.Reader, buf)
	fmt.Println(buf)
}
