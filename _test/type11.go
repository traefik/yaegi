package main

import (
	"compress/gzip"
	"fmt"
	"sync"
)

var gzipWriterPools [gzip.BestCompression - gzip.BestSpeed + 2]*sync.Pool

func main() {
	fmt.Printf("%T\n", gzipWriterPools)
}

// Output:
// [10]*sync.Pool
