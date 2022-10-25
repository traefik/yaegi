package main

type T uint16

func f() T { return 0 }

func main() {
	println(f())
}

// Output:
// 0
