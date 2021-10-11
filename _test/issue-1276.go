package main

import (
	"flag"
)

type customFlag struct{}

func (cf customFlag) String() string {
	return "custom flag"
}

func (cf customFlag) Set(string) error {
	return nil
}

func main() {
	flag.Var(customFlag{}, "cf", "custom flag")
	flag.Parse()
	println("Hello, playground")
}

// Output:
// Hello, playground
