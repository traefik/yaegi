package vm

import "syscall"

// platformSockopts are the socket options of darwin/arm64 that a plugin may
// set on a lent socket, on top of baseSockopts (rawconn_unix.go): the
// timing of TCP keepalives that the syscall package has there. TCP_KEEPALIVE
// is the idle time (the TCP_KEEPIDLE of linux; the net package of Go uses
// it for KeepAliveConfig.Idle on darwin). darwin/amd64 has only that one
// in syscall (sockopt_darwin_amd64.go).
var platformSockopts = []sockopt{
	{syscall.IPPROTO_TCP, syscall.TCP_KEEPALIVE},
	{syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL},
	{syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT},
}
