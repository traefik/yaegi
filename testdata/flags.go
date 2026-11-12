package main

import (
	"flag"
	"fmt"
	"os"
)

// The program owns os.Args and flag.CommandLine, not the interpreter.

var src = flag.String("src", "default", "a flag also defined by yaegi")

func main() {
	flag.Parse()
	fmt.Println(len(os.Args), flag.NArg(), *src)
}
