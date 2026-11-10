package main

func main() {
	println("hello")

	var r any = 1
	r = recover()
	if r == nil {
		println("world")
	}
}

// Output:
// hello
// world
