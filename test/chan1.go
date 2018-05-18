package main

func send(c chan<- string) { c <- "ping" }

func main() {
	channel := make(chan string)
	go send(channel)
	msg := <-channel
	println(msg)
}

// Output:
// ping
