package main

func main() {
	a, b := 1, 2

	if f2() && f1() {
		println(a, b)
	}
}

func f1() bool {
	println("f1")
	return true
}

func f2() bool {
	println("f2")
	return false
}

// Output:
// f2
