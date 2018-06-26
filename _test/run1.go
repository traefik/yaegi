package main

func f() (int, int) { return 2, 3 }

func g(i, j int) int { return i + j }

func main() {
	println(g(f()))
}

// Output:
// 5
