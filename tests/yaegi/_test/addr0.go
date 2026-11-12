package main

import (
	"fmt"
	"net/http"
)

type extendedRequest struct {
	http.Request

	Data string
}

func main() {
	r := extendedRequest{}
	req := &r.Request

	fmt.Printf("%T\n", r.Request)
	fmt.Printf("%T\n", req)
}

// Output:
// http.Request
// *http.Request
