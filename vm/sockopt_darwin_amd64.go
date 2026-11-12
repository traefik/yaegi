package vm

import "syscall"

// platformSockopts are the socket options of darwin/amd64 that a plugin may
// set on a lent socket, on top of baseSockopts (rawconn_unix.go):
// TCP_KEEPALIVE, the idle time of TCP keepalives (the TCP_KEEPIDLE of
// linux), the only one of the keepalive timing that the syscall package has
// there (darwin/arm64 also has TCP_KEEPINTVL and TCP_KEEPCNT,
// sockopt_darwin_arm64.go).
var platformSockopts = []sockopt{
	{syscall.IPPROTO_TCP, syscall.TCP_KEEPALIVE},
}
