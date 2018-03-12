package main

var channel chan string

func send() { channel <- "ping" }

func main() {
	channel = make(chan string)
	go send()
	msg := <-channel
	println(msg)
}
