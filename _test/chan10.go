package main

import "time"

func main() {
	var tick <-chan time.Time = time.Tick(time.Millisecond)
	_ = tick
	println("success")
}

// Output:
// success
