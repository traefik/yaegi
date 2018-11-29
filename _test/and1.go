package main

func main() {
	a := f2() && f1()
	println(a)
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
// false
