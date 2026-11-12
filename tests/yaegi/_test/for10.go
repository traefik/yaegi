package main

func main() {
	for a := 0; false; {
		println("nok", a)
		a++
		break
	}
	println("bye")
}

// Output:
// bye
