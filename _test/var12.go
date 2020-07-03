package main

var A = concat("hello", B)

var B = concat(" ", C, "!")

var C = D

var D = "world"

func concat(a ...string) string {
	var s string
	for _, ss := range a {
		s += ss
	}
	return s
}

func main() {
	println(A)
}

// Output:
// hello world!
