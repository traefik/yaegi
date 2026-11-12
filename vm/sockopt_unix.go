//go:build unix && !linux && !freebsd && !darwin

package vm

// platformSockopts: on the other unix systems, only baseSockopts
// (rawconn_unix.go).
var platformSockopts []sockopt
