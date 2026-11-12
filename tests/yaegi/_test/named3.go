package main

import "github.com/traefik/yaegi/_test/named3"

var globalT *T

func init() {
	globalT = &T{A: "test"}
}

type T named3.T

func (t *T) PrintT() {
	(*named3.T)(t).Print()
}

func main() {
	globalT.PrintT()
}

// Output:
// test
