package main

import (
	"fmt"
	"net/http"
)

type AuthenticatedRequest struct {
	http.Request
	Username string
}

func main() {
	a := &AuthenticatedRequest{}

	fmt.Printf("%v %T\n", a.Header, a.Header)
}

// Output:
// map[] http.Header
