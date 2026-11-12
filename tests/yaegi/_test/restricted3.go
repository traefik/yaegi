package main

import (
	"bytes"
	"fmt"
	"log"
)

var (
	buf    bytes.Buffer
	logger = log.New(&buf, "logger: ", log.Lshortfile)
)

func main() {
	defer func() {
		r := recover()
		fmt.Println("recover:", r, buf.String())
	}()
	logger.Fatal("test log")
}

// Output:
// recover: test log logger: restricted.go:39: test log
