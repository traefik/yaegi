package main

func main() {
	a, b := 1, 2
	a, b = b, -a
	println(a, b)
}

// Output:
// 2 -1
