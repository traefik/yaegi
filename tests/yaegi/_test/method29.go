package main

import (
	"context"
	"net"
)

var lookupHost = net.DefaultResolver.LookupHost

func main() {
	res, err := lookupHost(context.Background(), "localhost")
	println(len(res) > 0, err == nil)
}

// Output:
// true true
