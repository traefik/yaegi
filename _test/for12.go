package main

func main() {
	for a := 0; false; a++ {
		println("nok", a)
		break
	}
	println("bye")
}

// Output:
// bye
