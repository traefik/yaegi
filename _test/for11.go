package main

func main() {
	a := 0
	for ; true; a++ {
		println("nok", a)
		break
	}
	println("bye", a)
}

// Output:
// bye
