package vm

import "syscall"

// tcpUserTimeout is TCP_USER_TIMEOUT of linux (include/uapi/linux/tcp.h),
// the same number on every architecture. The syscall package has it only
// on some of them (not on amd64, 386 and arm); golang.org/x/sys/unix has it
// on all, and gRPC uses that one.
const tcpUserTimeout = 0x12

// platformSockopts are the socket options of linux that a plugin may set
// on a lent socket, on top of baseSockopts (rawconn_unix.go): the TCP user
// timeout (gRPC sets it on each connection of its server), the timing of
// TCP keepalives, and the quick ACK mode.
var platformSockopts = []sockopt{
	{syscall.IPPROTO_TCP, tcpUserTimeout},
	{syscall.IPPROTO_TCP, syscall.TCP_KEEPIDLE},
	{syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL},
	{syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT},
	{syscall.IPPROTO_TCP, syscall.TCP_QUICKACK},
}
