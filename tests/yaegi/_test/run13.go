package main

func main() {
	a, b := f(3)
	println(a, b)
}

func f(a int) (int, int) {
	return a + 1, a + 2
}

// Output:
// 4 5
