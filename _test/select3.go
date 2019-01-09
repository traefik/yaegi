package main

func main() {
	select {
	default:
		println("no comm")
	}
	println("bye")
}

// Output:
// no comm
// bye
