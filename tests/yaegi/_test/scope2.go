package main

var a int = 1

func f() { println(a) }

func main() {
	println(a)
	a := 2
	println(a)
	f()
}

// Output:
// 1
// 2
// 1
