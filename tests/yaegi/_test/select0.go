package main

import "time"

func forever() {
	select {} // block forever
	println("end")
}

func main() {
	go forever()
	time.Sleep(1e4)
	println("bye")
}

// Output:
// bye
