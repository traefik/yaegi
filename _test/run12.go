package main

func f(a int) (int, int) {
	return a + 1, a + 2
}

func main() {
	a, b := f(3)
	println(a, b)
}

// Output:
// 4 5
