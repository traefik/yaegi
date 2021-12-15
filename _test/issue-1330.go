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
	listener, err := net.Listen("tcp", "127.0.0.1:49153")
	if err != nil {
		panic(err)
	}
	go func() {
		_, err := listener.Accept()
		if err != nil {
			panic(err)
		}
	}()

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

	t := &wrappedConn{conn}
	var w io.Writer = t
	fmt.Println(w.Write != nil)
}

// Output:
// true
