//go:build unix

package vm

import (
	"fmt"
	"reflect"
	"syscall"
)

// sockLent reports whether a RawConn of the net package lends fd to the
// current goroutine.
func (m *Machine) sockLent(fd uintptr) bool {
	g := getg()
	m.fdMu.Lock()
	defer m.fdMu.Unlock()
	return m.lentFDs[lentFD{g, fd, true}] > 0
}

// lentRead is syscall.Read for the syscall subset (rawconn.go): it reads
// only a descriptor that a RawConn lends to the current goroutine, and
// panics with a *RefusedError for any other.
func lentRead(m *Machine) reflect.Value {
	return reflect.ValueOf(func(fd int, p []byte) (int, error) {
		if fd < 0 || !m.fdLent(uintptr(fd)) {
			panic(&RefusedError{What: "syscall.Read of a file descriptor that no RawConn callback lends to the plugin"})
		}
		return syscall.Read(fd, p)
	})
}

// recvfromFlags are the flags of Recvfrom in the subset: MSG_PEEK (look at
// the data without consuming it) and MSG_DONTWAIT (do not block).
const recvfromFlags = syscall.MSG_PEEK | msgDontwait

// checkRecvfrom panics with a *RefusedError, for the function fn
// ("syscall.Recvfrom"), unless fd is a socket that a RawConn of the net
// package lends to the current goroutine and flags has MSG_PEEK, and
// MSG_DONTWAIT at most besides: the call never consumes data. go-redis checks its pooled connections with
// Recvfrom(fd, buf, MSG_PEEK|MSG_DONTWAIT) in a RawConn.Read callback: it
// reads less than syscall.Read, which the subset has already, and its
// address result is the peer of the plugin's own connection.
func (m *Machine) checkRecvfrom(fn string, fd, flags int) {
	if fd < 0 || !m.sockLent(uintptr(fd)) {
		panic(&RefusedError{What: fn + " of a file descriptor that no RawConn callback of a network connection lends to the plugin"})
	}
	if flags&^recvfromFlags != 0 || flags&syscall.MSG_PEEK == 0 {
		panic(&RefusedError{What: fmt.Sprintf("%s with the flags %#x (MSG_PEEK, with MSG_DONTWAIT at most)", fn, flags)})
	}
}

// lentRecvfrom is syscall.Recvfrom for the syscall subset: only on a lent
// socket, with MSG_PEEK and MSG_DONTWAIT (checkRecvfrom).
func lentRecvfrom(m *Machine) reflect.Value {
	return reflect.ValueOf(func(fd int, p []byte, flags int) (int, syscall.Sockaddr, error) {
		m.checkRecvfrom("syscall.Recvfrom", fd, flags)
		return syscall.Recvfrom(fd, p, flags)
	})
}

// sockopt is a socket option: its level and its name.
type sockopt struct{ level, opt int }

// baseSockopts are the socket options of the syscall subset that every unix
// system has (sockopt_*.go add the others of the platform). Each one only
// tunes the connection of the plugin: Nagle's algorithm, TCP keepalives,
// the sizes of its buffers (the kernel bounds them for a process without
// privileges; the *FORCE options that pass the bound are not in the list).
var baseSockopts = []sockopt{
	{syscall.IPPROTO_TCP, syscall.TCP_NODELAY},
	{syscall.SOL_SOCKET, syscall.SO_KEEPALIVE},
	{syscall.SOL_SOCKET, syscall.SO_RCVBUF},
	{syscall.SOL_SOCKET, syscall.SO_SNDBUF},
}

// allowedSockopts are the socket options that a plugin may set and read on
// a lent socket (rawconn.go).
var allowedSockopts = func() map[sockopt]bool {
	out := map[sockopt]bool{}
	for _, o := range append(baseSockopts, platformSockopts...) {
		out[o] = true
	}
	return out
}()

// checkSockopt panics with a *RefusedError, for the function fn
// ("syscall.SetsockoptInt"), unless fd is a socket that a
// RawConn of the net package lends to the current goroutine and the option
// is in allowedSockopts.
func (m *Machine) checkSockopt(fn string, fd, level, opt int) {
	if fd < 0 || !m.sockLent(uintptr(fd)) {
		panic(&RefusedError{What: fn + " of a file descriptor that no RawConn callback of a network connection lends to the plugin"})
	}
	if !allowedSockopts[sockopt{level, opt}] {
		panic(&RefusedError{What: fmt.Sprintf("%s of the socket option %d of level %d (not in the allowed socket options)", fn, opt, level)})
	}
}

// lentSetsockoptInt is syscall.SetsockoptInt for the syscall subset: only
// on a lent socket and for an allowed option (checkSockopt).
func lentSetsockoptInt(m *Machine) reflect.Value {
	return reflect.ValueOf(func(fd, level, opt, value int) error {
		m.checkSockopt("syscall.SetsockoptInt", fd, level, opt)
		return syscall.SetsockoptInt(fd, level, opt, value)
	})
}

// lentGetsockoptInt is syscall.GetsockoptInt for the syscall subset: only
// on a lent socket and for an allowed option (checkSockopt).
func lentGetsockoptInt(m *Machine) reflect.Value {
	return reflect.ValueOf(func(fd, level, opt int) (int, error) {
		m.checkSockopt("syscall.GetsockoptInt", fd, level, opt)
		return syscall.GetsockoptInt(fd, level, opt)
	})
}
