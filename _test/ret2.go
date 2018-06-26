package main

func r2() (int, int) { return 1, 2 }

func main() {
	a, b := r2()
	println(a, b)
}

// Output:
// 1 2
