package main

// Methods promoted from an unexported native type: net.UDPConn embeds
// net.conn, which has Write, Read, SetDeadline, Close... The VM finds the
// native type from the exported ones and calls the methods through the
// unexported field.

import (
	"fmt"
	"net"
	"time"
)

func main() {
	srv, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		fmt.Println("listen:", err)
		return
	}
	defer srv.Close()
	cli, err := net.DialUDP("udp", nil, srv.LocalAddr().(*net.UDPAddr))
	if err != nil {
		fmt.Println("dial:", err)
		return
	}
	n, err := cli.Write([]byte("magic packet"))
	fmt.Println(n, err)
	srv.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 64)
	n, _, err = srv.ReadFromUDP(buf)
	fmt.Println(string(buf[:n]), err)
	fmt.Println(cli.Close(), cli.Close() != nil)
}
