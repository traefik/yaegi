package main

import (
	"fmt"
	"io"
	"net"
)

type wrappedConn struct {
	net.Conn
}

func main() {
	_, err := net.Listen("tcp", "127.0.0.1:49153")
	if err != nil {
		panic(err)
	}

	dialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 0,
		},
	}

	conn, err := dialer.Dial("tcp", "127.0.0.1:49153")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	t := &wrappedConn{conn}
	var w io.Writer = t
	if n, err := w.Write([]byte("hello")); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(n)
	}
}

// Output:
// 5
