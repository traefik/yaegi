package main

import "github.com/traefik/yaegi/_test/alias3"

var globalT *T

func init() {
	globalT = &T{A: "test"}
}

type T alias3.T

func (t *T) PrintT() {
	(*alias3.T)(t).Print()
}

func main() {
	globalT.PrintT()
}

// Output:
// test
