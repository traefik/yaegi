package main

type Channel chan string

func send(c Channel) { c <- "ping" }

func main() {
	channel := make(Channel)
	go send(channel)
	msg := <-channel
	println(msg)
}

// Output:
// ping
