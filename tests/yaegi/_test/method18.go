package main

import (
	"compress/gzip"
	"fmt"
	"net/http"
)

type GzipResponseWriter struct {
	http.ResponseWriter
	index int
	gw    *gzip.Writer
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
