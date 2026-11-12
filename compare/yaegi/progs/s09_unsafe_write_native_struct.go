package main

import (
	"fmt"
	"net/http"
	"unsafe"
)

type fakeRequest struct {
	Method string
}

// Plugin overwriting a host-owned (native) object's memory through unsafe:
// the kind of access the useUnsafe opt-in is meant to gate.
func main() {
	r, _ := http.NewRequest("GET", "http://x", nil)
	(*fakeRequest)(unsafe.Pointer(r)).Method = "HACKED"
	fmt.Println(r.Method)
}
