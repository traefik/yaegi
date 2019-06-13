package main

import "net/http"

type T struct {
	header string
}

func (b *T) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if b.header != "" {
		req.Header[b.header] = []string{"hello"}
	}
}

func main() {
	println("ok")
}

// Output:
// ok
