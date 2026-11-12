// A program that uses syscall.RawConn as gRPC does: the Control function of
// a net.Dialer (grpc's keepalive dialer) gets the RawConn of the new socket,
// and the reads of the connection go through RawConn.Read and syscall.Read
// (internal/transport/readyreader on linux). Run in default mode, and as a
// plugin with useUnsafe (-restricted -unsafe): there the RawConn methods
// lend the file descriptor to the callback, and syscall.Read reads it.
package main

import (
	"fmt"
	"net"
	"syscall"
)

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("listen:", err)
		return
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		c.Write([]byte("hello through RawConn"))
		c.Close()
	}()

	controlled := false
	d := net.Dialer{Control: func(network, address string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) { controlled = fd > 2 })
	}}
	conn, err := d.Dial("tcp", ln.Addr().String())
	if err != nil {
		fmt.Println("dial:", err)
		return
	}
	defer conn.Close()

	sc, ok := conn.(syscall.Conn)
	if !ok {
		fmt.Println("not a syscall.Conn")
		return
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		fmt.Println("SyscallConn:", err)
		return
	}
	var got []byte
	buf := make([]byte, 8)
	for {
		var n int
		var rerr error
		err := raw.Read(func(fd uintptr) bool {
			n, rerr = syscall.Read(int(fd), buf)
			return rerr != syscall.EAGAIN && rerr != syscall.EWOULDBLOCK
		})
		if err != nil || rerr != nil {
			fmt.Println("read:", err, rerr)
			break
		}
		if n <= 0 {
			break
		}
		got = append(got, buf[:n]...)
	}
	fmt.Printf("control=%v read=%q\n", controlled, got)
}
