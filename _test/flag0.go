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
// 0
