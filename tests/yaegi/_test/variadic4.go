package main

func variadic(s ...string) {}

func f(s string) { println(s + "bar") }

func main() { f("foo") }

// Output:
// foobar
