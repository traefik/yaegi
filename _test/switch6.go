package main

func main() {
	a := 3
	switch b := "foo"; {
	case a == 0:
		println(200)
	case a == 3:
		println(100)
		fallthrough
	default:
		println(a, b)
	}
}

// Output:
// 100
// 3 foo
