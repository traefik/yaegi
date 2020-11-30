package main

import "time"

type TimeValue time.Time

func (v *TimeValue) decode() { println("in decode") }

func main() {
	var tv TimeValue
	tv.decode()
}

// Output:
// in decode
