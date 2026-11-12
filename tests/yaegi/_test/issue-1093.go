package main

func b() string {
	return "b"
}

func main() {
	var x int
	x = "a" + b()
}

// Error:
// 9:6: cannot use type untyped string as type int in assignment
