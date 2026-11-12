package main

func init() {
	println("Hello from init 1")
}

func init() {
	println("Hello from init 2")
}

func main() {
	println("Hello from main")
}

// Output:
// Hello from init 1
// Hello from init 2
// Hello from main
