package main

import "strings"

func f(s string) string { return "hello " + s }

func g(s string) string { return "hi " + s }

var methods = map[string]func(string) string{"f": f}

func main() {
	methods["i"] = strings.ToUpper
	methods["g"] = g
	println(methods["f"]("test"))
	println(methods["g"]("test"))
	println(methods["i"]("test"))
}

// Output:
// hello test
// hi test
// TEST
