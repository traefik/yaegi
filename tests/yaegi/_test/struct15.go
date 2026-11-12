package main

import (
	"fmt"
	"net/http"
)

type GzipResponseWriter struct {
	http.ResponseWriter
	index int
}

type GzipResponseWriterWithCloseNotify struct {
	*GzipResponseWriter
}

func (w GzipResponseWriterWithCloseNotify) CloseNotify() <-chan bool {
	return w.ResponseWriter.(http.CloseNotifier).CloseNotify()
}

func main() {
	fmt.Println("hello")
}

// Output:
// hello
