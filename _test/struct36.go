package main

import (
	"fmt"
	"net/http"
)

type S struct {
	http.Client
}

func main() {
	var s S
	_, err := s.Get("url")
	fmt.Println(err)
	return
}

// Output:
// Get "url": unsupported protocol scheme ""
