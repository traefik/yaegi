package main

func main() {
	const (
		x = 2 * iota
		dim
	)
	var t [dim * 2]int
	println(t[0], len(t))
}

// Output:
// 0 4
