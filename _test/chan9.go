package main

type Channel chan string

type T struct {
	Channel
}

func send(c Channel) { c <- "ping" }

func main() {
	t := &T{}
	t.Channel = make(Channel)
	go send(t.Channel)
	msg := <-t.Channel
	println(msg)
}

// Output:
// ping
