package main

type T uint

func main() {
	type myint int
	var i = myint(1)
	println(i)
}

// Output:
// 1
