package main

var time int

type time string

func main() {
	var t time = "hello"
	println(t)
}

// TODO: expected redeclaration error.
