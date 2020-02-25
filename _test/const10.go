package main

const (
	a = 2
	b = c + d
	c = a + d
	d = e + f
	e = 3
	f = 4
)

func main() {
	println(b)
}

// Output:
// 16
