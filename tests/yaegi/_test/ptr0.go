package main

type myint int

func main() {
	var a myint = 2
	var b *myint = &a
	println(*b)
}

// Output:
// 2
