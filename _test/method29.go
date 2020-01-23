package main

import (
	"context"
	"fmt"
	"net"
)

var lookupHost = net.DefaultResolver.LookupHost

func main() {
	fmt.Println(lookupHost(context.Background(), "localhost"))
}

// Output:
// [::1 127.0.0.1] <nil>
