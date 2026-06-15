package main

import (
	"compress/gzip"
	"fmt"
	"sync"
)

var gzipWriterPools [10]*sync.Pool = [10]*sync.Pool{}

func main() {
	level := 9
	gzipWriterPools[level] = &sync.Pool{
		New: func() any {
			w, _ := gzip.NewWriterLevel(nil, level)
			return w
		},
	}
	gzw := gzipWriterPools[level].Get().(*gzip.Writer)
	fmt.Printf("gzw: %T\n", gzw)
}

// Output:
// gzw: *gzip.Writer
