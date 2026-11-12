package main

func main() {
	println("hello")

	var r interface{} = 1
	r = recover()
	if r == nil {
		println("world")
	}
}

// Output:
// hello
// world
