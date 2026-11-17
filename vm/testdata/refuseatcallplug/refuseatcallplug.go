// Package refuseatcallplug names refused functions in code that it runs
// only on demand, as big dependency graphs do (caddy, go-systemd, olric):
// refused at load time by default, loaded with Options.RefuseAtCall.
package refuseatcallplug

import (
	"net"
	"net/http/pprof"
	"os"
	"os/signal"
	"plugin"
	"runtime/debug"
)

// Fine calls no refused function.
func Fine() bool { return true }

// FindProcess calls a refused function of an allowed package.
func FindProcess() error {
	_, err := os.FindProcess(1)
	return err
}

// MemoryLimit calls a refused function with no error result.
func MemoryLimit() int64 { return debug.SetMemoryLimit(-1) }

// Ignore calls a refused function in a defer.
func Ignore() bool {
	defer signal.Ignore()
	return true
}

// Value takes a refused function as a value, without calling it.
func Value() bool {
	f := os.FindProcess
	return f != nil
}

// CallValue calls a refused function through a value.
func CallValue() error {
	f := os.FindProcess
	_, err := f(1)
	return err
}

// WriteMsg calls a refused method (refusedmeth.go).
func WriteMsg() error {
	var c *net.UnixConn
	_, _, err := c.WriteMsgUnix([]byte("x"), nil, nil)
	return err
}

// Open calls a function of package plugin.
func Open() error {
	_, err := plugin.Open("x.so")
	return err
}

// Lookup calls a method of package plugin.
func Lookup() error {
	var p *plugin.Plugin
	_, err := p.Lookup("X")
	return err
}

// Pprof calls a function of package net/http/pprof.
func Pprof() bool { return pprof.Handler("goroutine") != nil }
