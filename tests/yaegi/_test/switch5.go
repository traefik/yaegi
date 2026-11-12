package main

func main() {
	i := 1

	switch i {
	case 0, 1, 2:
		if i == 1 {
			println("one")
			break
		}
		println(i)
	default:
		println("not nul")
	}
}

// Output:
// one
