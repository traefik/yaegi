package main

func send(c chan<- int32) { c <- 123 }

func main() {
	channel := make(chan int32)
	go send(channel)
	msg := <-channel
	println(msg)
}

// Output:
// 123
