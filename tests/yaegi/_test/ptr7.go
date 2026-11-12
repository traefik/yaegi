package main

import (
	"fmt"
	"net"
	"strings"
)

type ipNetValue net.IPNet

func (ipnet *ipNetValue) Set(value string) error {
	_, n, err := net.ParseCIDR(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	*ipnet = ipNetValue(*n)
	return nil
}

func main() {
	v := ipNetValue{}
	fmt.Println(v)
}

// Output:
// {<nil> <nil>}
