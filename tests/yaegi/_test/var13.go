package main

var (
	a = concat("hello", b)
	b = concat(" ", c, "!")
	c = d
	d = "world"
)

func concat(a ...string) string {
	var s string
	for _, ss := range a {
		s += ss
	}
	return s
}

func main() {
	println(a)
}

// Output:
// hello world!
