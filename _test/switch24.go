package main

func main() {
	a := 3
	switch a + 2 {
	case 5:
		println(5)
	default:
		println("default")
	}
	println("bye")
}

// Output:
// 5
// bye
