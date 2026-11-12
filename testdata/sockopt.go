// A program that sets socket options on its own TCP connection, as gRPC
// does (TCP_USER_TIMEOUT on linux, in RawConn.Control), and reads them back
// with GetsockoptInt. Run in default mode, and as a plugin with useUnsafe
// (-restricted -unsafe): there SetsockoptInt and GetsockoptInt take the
// descriptor that the RawConn lends to the callback, for a short list of
// options. The connection still works after.
package main

import (
	"fmt"
	"net"
	"runtime"
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
		buf := make([]byte, 4)
		n, _ := c.Read(buf)
		c.Write([]byte("pong " + string(buf[:n])))
		c.Close()
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		fmt.Println("dial:", err)
		return
	}
	defer conn.Close()
	raw, err := conn.(*net.TCPConn).SyscallConn()
	if err != nil {
		fmt.Println("SyscallConn:", err)
		return
	}

	type opt struct {
		name         string
		level, opt   int
		value        int
		readBackSame bool // the kernel gives back the value that was set
	}
	opts := []opt{
		{"TCP_NODELAY off", syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 0, true},
		{"TCP_NODELAY on", syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1, false},
		{"SO_KEEPALIVE on", syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1, false},
		{"SO_KEEPALIVE off", syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 0, true},
		{"SO_RCVBUF", syscall.SOL_SOCKET, syscall.SO_RCVBUF, 65536, false},
		{"SO_SNDBUF", syscall.SOL_SOCKET, syscall.SO_SNDBUF, 65536, false},
	}
	if runtime.GOOS == "linux" {
		// The numbers of linux (the syscall package has no
		// TCP_USER_TIMEOUT on amd64).
		opts = append(opts,
			opt{"TCP_USER_TIMEOUT", syscall.IPPROTO_TCP, 0x12, 20000, true},
			opt{"TCP_KEEPIDLE", syscall.IPPROTO_TCP, 4, 30, true},
			opt{"TCP_KEEPINTVL", syscall.IPPROTO_TCP, 5, 10, true},
			opt{"TCP_KEEPCNT", syscall.IPPROTO_TCP, 6, 3, true},
		)
	}
	if runtime.GOOS == "darwin" {
		// TCP_KEEPALIVE of darwin: the idle time of keepalives (the
		// TCP_KEEPIDLE of linux), on amd64 and arm64.
		opts = append(opts, opt{"TCP_KEEPALIVE", syscall.IPPROTO_TCP, 0x10, 30, true})
	}
	err = raw.Control(func(fd uintptr) {
		for _, o := range opts {
			serr := syscall.SetsockoptInt(int(fd), o.level, o.opt, o.value)
			v, gerr := syscall.GetsockoptInt(int(fd), o.level, o.opt)
			if o.readBackSame {
				fmt.Printf("%s: set %v, get %v %v\n", o.name, serr, v == o.value, gerr)
			} else {
				fmt.Printf("%s: set %v, get nonzero %v %v\n", o.name, serr, v != 0, gerr)
			}
		}
	})
	fmt.Println("control:", err)

	conn.Write([]byte("ping"))
	buf := make([]byte, 16)
	n, err := conn.Read(buf)
	fmt.Printf("conn: %q %v\n", buf[:n], err)
}
