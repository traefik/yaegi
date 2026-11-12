package main

import "net"

func main() {
	c := append(net.Buffers{}, []byte{})
	println(len(c))
}

// Output:
// 1
