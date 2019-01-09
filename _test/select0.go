package main

import "time"

func forever() {
	select {} // block forever
	println("end")
}

func main() {
	go forever()
	time.Sleep(1e9)
	println("bye")
}
