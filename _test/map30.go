package main

import "strings"

func f(s string) string { return "hello " + s }

var methods = map[string]func(string) string{"f": f}

func main() {
	methods["g"] = strings.ToUpper
	println(methods["f"]("test"))
	println(methods["g"]("test"))
}

// Output:
// hello test
// TEST
