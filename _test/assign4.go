package main

func main() {
	a, b, c := 1, 2, 3
	println(a, b, c)
	a, b, c = c, a, b
	println(a, b, c)
}

// Output:
// 1 2 3
// 3 1 2
