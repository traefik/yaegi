package main

import (
	"flag"
	"fmt"
)

func main() {
	flag.Parse()
	fmt.Println("Narg:", flag.NArg())
}

// Output:
// Narg: 0
