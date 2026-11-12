package main

func f(a int) int {
	return 2*a + 1
}

var b int = f(3)

func main() {
	println(b)
}

// Output:
// 7
