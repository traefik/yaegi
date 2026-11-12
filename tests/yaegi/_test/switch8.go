package main

func main() {
	println("hello")
	fallthrough
	println("world")
}

// Error:
// 5:2: fallthrough statement out of place
