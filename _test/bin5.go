package main

import (
	"fmt"
	"net"
)

func main() {
	addr := net.TCPAddr{IP: net.IPv4(1, 1, 1, 1), Port: 80}
	var s fmt.Stringer = &addr
	fmt.Println(s.String())
}

// Output:
// 1.1.1.1:80
