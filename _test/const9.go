package main

const (
	a = 2
	b = c + d
	c = a + d
	d = e + f
	e = b + 2
	f = 4
)

func main() {
	println(b)
}

// Error:
// 5:2: constant definition loop
