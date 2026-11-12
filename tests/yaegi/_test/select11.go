package main

func main() {
	c := make(chan string)
	select {
	case <-c:
		println("unexpected")
	default:
		println("nothing received")
	}
	println("bye")
}

// Output:
// nothing received
// bye
