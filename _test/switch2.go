package main

func main() {
	i := 1

	switch i {
	case 0, 1, 2:
		println(i)
	default:
		println("not nul")
	}
}

// Output:
// 1
