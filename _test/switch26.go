package main

func main() {
	a := 1
	switch a := 2; {
	case a == 1:
		println(1)
	case a == 2:
		println(2)
	default:
		println("default")
	}
	println(a)
}

// Output:
// 2
// 1
