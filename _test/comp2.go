package main

type delta int32

func main() {
	a := delta(-1)

	println(a != -1)
	println(a == -1)
}

// Output:
// false
// true
