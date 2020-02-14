package main

func main() {
	messages := make(chan bool)

	go func() { messages <- true }()

	println(<-messages && true)
}

// Output:
// true
