package main

import (
	"fmt"
)

var _ = (HelloInterface)((*Hello)(nil))

type HelloInterface interface {
	Hi() string
}

type Hello struct{}

func (h *Hello) Hi() string {
	return "hi"
}

func main() {
	h := &Hello{}
	fmt.Println(h.Hi())
}

// Output:
// hi
