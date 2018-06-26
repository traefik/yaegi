package main

func main() {
	a, b := 1, 2

	if f1() && f2() {
		println(a, b)
	}
}

func f1() bool {
	println("f1")
	//return true
	return 0 == 0
}

func f2() bool {
	println("f2")
	//return false
	return 1 == 0
}

// Output:
// f1
// f2
