package main

func main() {
	a := 0
	for ; false; a++ {
		println("nok", a)
		break
	}
	println("bye", a)
}

// Output:
// bye 0
