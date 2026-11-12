package main

func send(c chan<- bool) { c <- false }

func main() {
	channel := make(chan bool)
	go send(channel)
	if <-channel {
		println("ok")
	} else {
		println("nok")
	}
}

// Output:
// nok
