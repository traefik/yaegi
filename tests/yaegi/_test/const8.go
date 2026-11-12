package main

const (
	a = 2
	b = c + d
	c = 4
	d = 5
)

func main() {
	println(a, b, c, d)
}

// Output:
// 2 9 4 5
