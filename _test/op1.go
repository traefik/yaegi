package main

func main() {
	var a int = 3
	a += 1.3
	println(a)
}

// Error:
// 5:2: invalid operation: mismatched types int and untyped float
