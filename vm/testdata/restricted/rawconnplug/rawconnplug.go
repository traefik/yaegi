// Package rawconnplug gets the syscall.RawConn of its own TCP connection
// and tries to use the file descriptor outside of what the syscall subset
// of a plugin with useUnsafe allows: read it outside the callback, on
// another goroutine, in the callback of another connection, after a panic
// of the callback, read a descriptor of the host, close it, write to it,
// set a socket option outside the allowed list, outside the callback or on
// a file, make an *os.File of it with os.NewFile, lend it through a RawConn
// of its own. It also sets and reads back the allowed socket options in the
// callback, peeks at the socket with syscall.Recvfrom in a Read callback as
// go-redis does (and with other flags, outside the callback, on another
// goroutine, on a file, on a descriptor of the host), reads the descriptor
// by the ways that lend it (a method value,
// reflect), while another goroutine closes the connection, and gives a
// syscall.Conn of its own to native code. Each request answers the result
// of one try, and then whether the connection still works (nothing was
// closed or written).
package rawconnplug

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"reflect"
	"syscall"
	"time"
)

type Config struct{}

func CreateConfig() *Config { return &Config{} }

// pair returns a TCP connection to an echo server, with its RawConn.
func pair() (net.Conn, syscall.RawConn, func()) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		io.Copy(c, c)
		c.Close()
	}()
	conn, _ := net.Dial("tcp", ln.Addr().String())
	raw, _ := conn.(*net.TCPConn).SyscallConn()
	return conn, raw, func() { conn.Close(); ln.Close() }
}

// ownRaw is a RawConn of the plugin: it gives its callback the descriptor
// of its choice. Native code must never call it, and it lends nothing.
type ownRaw struct{ called *int }

func (r ownRaw) Control(f func(uintptr)) error    { *r.called++; f(0); return nil }
func (r ownRaw) Read(f func(uintptr) bool) error  { *r.called++; f(0); return nil }
func (r ownRaw) Write(f func(uintptr) bool) error { *r.called++; f(0); return nil }

// ownConn is a reader of the plugin that is also a syscall.Conn, as the
// zero-copy paths of net and os look for.
type ownConn struct {
	data   string
	called *int
}

func (c *ownConn) Read(p []byte) (int, error) {
	if c.data == "" {
		return 0, io.EOF
	}
	n := copy(p, c.data)
	c.data = c.data[n:]
	return n, nil
}

func (c *ownConn) SyscallConn() (syscall.RawConn, error) { return ownRaw{c.called}, nil }

// try runs f and returns its panic, or "ok".
func try(f func()) (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprint(r)
		}
	}()
	f()
	return "ok"
}

func New(_ context.Context, _ http.Handler, _ *Config, _ string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			fmt.Fprint(w, "listen: ", err)
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
			c.Write([]byte("pong" + string(buf[:n])))
			c.Close()
		}()
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			fmt.Fprint(w, "dial: ", err)
			return
		}
		defer conn.Close()
		raw, err := conn.(*net.TCPConn).SyscallConn()
		if err != nil {
			fmt.Fprint(w, "SyscallConn: ", err)
			return
		}
		var fd int
		raw.Control(func(f uintptr) { fd = int(f) })
		buf := make([]byte, 16)

		var res string
		switch r.URL.Path {
		case "/outside":
			// the number of the socket, after the callback
			res = try(func() { syscall.Read(fd, buf) })
		case "/host":
			// a descriptor of the host, inside the callback
			raw.Control(func(uintptr) { res = try(func() { syscall.Read(0, buf) }) })
		case "/goroutine":
			// the lent descriptor, on another goroutine
			raw.Control(func(f uintptr) {
				done := make(chan string)
				go func() { done <- try(func() { syscall.Read(int(f), buf) }) }()
				res = <-done
			})
		case "/close":
			raw.Control(func(f uintptr) { res = try(func() { syscall.Close(int(f)) }) })
		case "/write":
			raw.Control(func(f uintptr) { res = try(func() { syscall.Write(int(f), []byte("x")) }) })
		case "/setsockopt":
			// a Setsockopt function outside the subset
			raw.Control(func(f uintptr) {
				res = try(func() { syscall.SetsockoptString(int(f), syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, "x") })
			})
		case "/sockopt":
			// allowed options on the lent socket: set, then read back
			var out []string
			raw.Control(func(f uintptr) {
				res = try(func() {
					for _, o := range [][3]int{
						{syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 0},
						{syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1},
						{syscall.SOL_SOCKET, syscall.SO_SNDBUF, 65536},
					} {
						err := syscall.SetsockoptInt(int(f), o[0], o[1], o[2])
						v, gerr := syscall.GetsockoptInt(int(f), o[0], o[1])
						out = append(out, fmt.Sprintf("%v/%v/%v", err, v != 0, gerr))
					}
				})
			})
			res += fmt.Sprint(out)
		case "/sockopt-outside":
			// the number of the socket, after the callback
			res = try(func() { syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1) })
		case "/getsockopt-outside":
			res = try(func() { syscall.GetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_NODELAY) })
		case "/sockopt-goroutine":
			raw.Control(func(f uintptr) {
				done := make(chan string)
				go func() {
					done <- try(func() { syscall.SetsockoptInt(int(f), syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1) })
				}()
				res = <-done
			})
		case "/sockopt-host":
			// a descriptor of the host, inside the callback
			raw.Control(func(uintptr) {
				res = try(func() { syscall.SetsockoptInt(1, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1) })
			})
		case "/sockopt-other":
			// the lent socket, in the callback of another connection
			_, raw2, close2 := pair()
			defer close2()
			raw2.Control(func(uintptr) {
				res = try(func() { syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1) })
			})
		case "/sockopt-option":
			// an option outside the list, on the lent socket
			raw.Control(func(f uintptr) {
				res = try(func() { syscall.SetsockoptInt(int(f), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1) })
			})
		case "/getsockopt-option":
			raw.Control(func(f uintptr) {
				res = try(func() { syscall.GetsockoptInt(int(f), syscall.SOL_SOCKET, syscall.SO_BROADCAST) })
			})
		case "/sockopt-level":
			// an allowed option number at another level
			raw.Control(func(f uintptr) {
				res = try(func() { syscall.SetsockoptInt(int(f), syscall.IPPROTO_IP, syscall.TCP_NODELAY, 1) })
			})
		case "/sockopt-file":
			// the descriptor of a file (a RawConn of os): Read only
			pr, pw, err := os.Pipe()
			if err != nil {
				res = err.Error()
				break
			}
			defer pr.Close()
			defer pw.Close()
			praw, _ := pr.SyscallConn()
			praw.Control(func(f uintptr) {
				res = try(func() { syscall.SetsockoptInt(int(f), syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1) })
			})
		case "/recvfrom":
			// go-redis checks a pooled connection: no data is waiting, so
			// the peek gives EAGAIN and consumes nothing
			raw.Read(func(f uintptr) bool {
				var eagain bool
				res = try(func() {
					var b [1]byte
					_, _, err := syscall.Recvfrom(int(f), b[:], syscall.MSG_PEEK|syscall.MSG_DONTWAIT)
					eagain = err == syscall.EAGAIN
				})
				res += fmt.Sprintf(" eagain=%v", eagain)
				return true
			})
		case "/recvfrom-flags":
			// a flag that consumes or waits for the data (MSG_WAITALL)
			raw.Read(func(f uintptr) bool {
				res = try(func() { syscall.Recvfrom(int(f), buf, syscall.MSG_PEEK|syscall.MSG_WAITALL) })
				return true
			})
		case "/recvfrom-nopeek":
			// without MSG_PEEK the call consumes data
			raw.Read(func(f uintptr) bool {
				res = try(func() { syscall.Recvfrom(int(f), buf, 0) })
				return true
			})
		case "/recvfrom-outside":
			res = try(func() { syscall.Recvfrom(fd, buf, syscall.MSG_PEEK|syscall.MSG_DONTWAIT) })
		case "/recvfrom-goroutine":
			raw.Read(func(f uintptr) bool {
				done := make(chan string)
				go func() {
					done <- try(func() { syscall.Recvfrom(int(f), buf, syscall.MSG_PEEK|syscall.MSG_DONTWAIT) })
				}()
				res = <-done
				return true
			})
		case "/recvfrom-host":
			raw.Read(func(uintptr) bool {
				res = try(func() { syscall.Recvfrom(0, buf, syscall.MSG_PEEK|syscall.MSG_DONTWAIT) })
				return true
			})
		case "/recvfrom-file":
			// the descriptor of a file (a RawConn of os) is not a socket
			pr, pw, err := os.Pipe()
			if err != nil {
				res = err.Error()
				break
			}
			defer pr.Close()
			defer pw.Close()
			praw, _ := pr.SyscallConn()
			praw.Control(func(f uintptr) {
				res = try(func() { syscall.Recvfrom(int(f), buf, syscall.MSG_PEEK|syscall.MSG_DONTWAIT) })
			})
		case "/fstatfs":
			// the statistics of the file system of a lent file (darwin gives
			// EINVAL for a socket)
			tf, err := os.CreateTemp("", "fstatfs")
			if err != nil {
				res = err.Error()
				break
			}
			defer os.Remove(tf.Name())
			defer tf.Close()
			traw, _ := tf.SyscallConn()
			traw.Control(func(f uintptr) {
				var st syscall.Statfs_t
				var err error
				res = try(func() { err = syscall.Fstatfs(int(f), &st) })
				// the fields differ per system: only check that the call
				// filled the struct
				res += fmt.Sprintf(" err=%v filled=%v", err, st != syscall.Statfs_t{})
			})
		case "/fstatfs-host":
			// a descriptor of the host, inside the callback
			raw.Control(func(uintptr) {
				var st syscall.Statfs_t
				res = try(func() { syscall.Fstatfs(0, &st) })
			})
		case "/fstatfs-outside":
			var st syscall.Statfs_t
			res = try(func() { syscall.Fstatfs(fd, &st) })
		case "/newfile":
			// an *os.File of a host descriptor number
			res = try(func() { os.NewFile(1, "stdout") })
		case "/newfile-value":
			newFile := os.NewFile
			res = try(func() { newFile(uintptr(fd), "socket") })
		case "/other":
			// the lent descriptor, in the callback of another connection
			_, raw2, close2 := pair()
			defer close2()
			raw2.Control(func(uintptr) { res = try(func() { syscall.Read(fd, buf) }) })
		case "/panic":
			// after a callback that panicked
			func() {
				defer func() { recover() }()
				raw.Control(func(uintptr) { panic("x") })
			}()
			res = try(func() { syscall.Read(fd, buf) })
		case "/own":
			// a RawConn of the plugin lends nothing
			var rc syscall.RawConn = ownRaw{new(int)}
			rc.Control(func(f uintptr) { res = try(func() { syscall.Read(int(f), buf) }) })
		case "/methodvalue":
			ctl := raw.Control
			ctl(func(f uintptr) { res = try(func() { syscall.Read(int(f), buf) }) })
		case "/reflect":
			cb := func(f uintptr) { res = try(func() { syscall.Read(int(f), buf) }) }
			reflect.ValueOf(raw).MethodByName("Control").Call([]reflect.Value{reflect.ValueOf(cb)})
		case "/closed":
			// Another goroutine closes the connection during the callback:
			// the descriptor stays open until the callback returns, so a
			// new connection gets another number, which is not lent.
			conn2, raw2, close2 := pair()
			defer close2()
			raw2.Control(func(f uintptr) {
				go conn2.Close()
				time.Sleep(50 * time.Millisecond)
				_, raw3, close3 := pair()
				defer close3()
				var fd3 int
				raw3.Control(func(f3 uintptr) { fd3 = int(f3) })
				res = fmt.Sprintf("reuse=%v new=%s", fd3 == int(f), try(func() { syscall.Read(fd3, buf) }))
			})
		case "/ownconn":
			// A syscall.Conn of the plugin given to native code (the
			// ReadFrom of a TCPConn, which looks for one on linux): the
			// copy goes through Read, native code never calls the RawConn.
			called := 0
			conn2, _, close2 := pair()
			defer close2()
			n, err := io.Copy(conn2, &ownConn{data: "hello", called: &called})
			echo := make([]byte, 5)
			io.ReadFull(conn2, echo)
			res = fmt.Sprintf("n=%d err=%v called=%d echo=%s", n, err, called, echo)
		}
		// The connection still works: the plugin closed and wrote nothing.
		conn.Write([]byte("ping"))
		n, err := conn.Read(buf)
		fmt.Fprintf(w, "%s | conn=%q %v", res, buf[:n], err)
	}), nil
}
