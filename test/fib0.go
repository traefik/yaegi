package main

// Compute fibonacci numbers, no memoization
func fib(n int) int {
	if n < 2 {
		return n
	}
	return fib(n-2) + fib(n-1)
}

func main() {
	println(fib(4))
}

// Output:
// 3
