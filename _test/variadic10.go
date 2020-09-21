package main

import (
	"log"
	"os"
)

func main() {
	logger := log.New(os.Stdout, "test ", log.Lmsgprefix)
	logger.Printf("args: %v %v", 1, "truc")
}

// Output:
// test args: 1 truc
