package main

func main() {
	c := make(chan string)
	select {
	case <-c:
		println("unexpected")
	default:
	}
	println("bye")
}

// Output:
// bye
