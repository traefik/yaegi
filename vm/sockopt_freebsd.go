package vm

import "syscall"

// platformSockopts are the socket options of freebsd that a plugin may set
// on a lent socket, on top of baseSockopts (rawconn_unix.go): the timing of
// TCP keepalives.
var platformSockopts = []sockopt{
	{syscall.IPPROTO_TCP, syscall.TCP_KEEPIDLE},
	{syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL},
	{syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT},
}
