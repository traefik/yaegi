package main

import (
	"fmt"
	"net/http"
)

func main() {
	next := func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Cache-Control", "max-age=20")
		rw.WriteHeader(http.StatusOK)
	}
	f := http.HandlerFunc(next)
	fmt.Printf("%T\n", f.ServeHTTP)
}

// Output:
// func(http.ResponseWriter, *http.Request)
