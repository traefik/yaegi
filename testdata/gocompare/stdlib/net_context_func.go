// A Context of the program whose Value gives a func: net asserts the value
// of its alternate resolver key to the func type of the resolver. With -aot
// the func in an any was an opaque handle, so net did not use it. (A named
// func type of the program is not that func type in Go; without -aot the
// VM gives native code the underlying func: a documented limit.)
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
)

type altCtx struct {
	context.Context
	v any
}

func (c altCtx) Value(key any) any { return c.v }

func main() {
	r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return nil, errors.New("no dns")
	}}
	alt := func(ctx context.Context, network, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.IPv4(203, 0, 113, 5)}}, nil
	}
	ips, err := r.LookupIPAddr(altCtx{context.Background(), alt}, "h.example.")
	fmt.Println(ips, err)
	_, err = r.LookupIPAddr(altCtx{context.Background(), "not a func"}, "h.example.")
	fmt.Println(err != nil)
}
