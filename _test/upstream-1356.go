package main

import (
	"fmt"
	"net"
	"time"
)

type Holder struct {
	F func() string
}

func main() {
	d := &net.Dialer{Timeout: time.Second}
	g := d.DialContext
	fmt.Println("dialer method value:", g != nil)
	h := Holder{F: (&time.Location{}).String}
	fmt.Println("literal:", h.F != nil)
}

// Output:
// dialer method value: true
// literal: true
