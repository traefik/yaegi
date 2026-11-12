package main

func main() {
	a, b := 1, 2
	println(a, b)
	a, b = b, a
	println(a, b)
}

// Output:
// 1 2
// 2 1
