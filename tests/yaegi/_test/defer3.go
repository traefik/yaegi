package main

import (
	"net/http"
	"net/http/httptest"
)

func main() {
	println("hello")
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
}

// Output:
// hello
