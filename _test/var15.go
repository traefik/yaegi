package main

var a int = 2

func inca() {
	a = a + 1
}

func main() {
	inca()
	println(a)
}

// Output:
// 3
