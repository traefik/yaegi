package main

type Channel chan string

var channel Channel

func send() { channel <- "ping" }

func main() {
	channel = make(Channel)
	go send()
	msg := <-channel
	println(msg)
}
