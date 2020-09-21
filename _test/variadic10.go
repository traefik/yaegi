package main

import (
	"log"
	"os"
)

func main() {
	logger := log.New(os.Stdout, "test ", log.Lmsgprefix)
	logger.Printf("args: %v %v", 1, "truc")
	logger.Printf("args: %v %v %v", 1, "truc", 2)
}

// Output:
// test args: 1 truc
// test args: 1 truc 2
