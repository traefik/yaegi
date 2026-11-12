package main

func main() {
	a := 2
	switch {
	case a == 1:
		println(1)
	case a == 2:
		println(2)
	default:
		println("default")
	}
	println("bye")
}

// Output:
// 2
// bye
