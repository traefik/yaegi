package main

type Level int8

const (
	a Level = -1
	b Level = 5
	d       = b - a + 1
)

type counters [d]int

func main() {
	println(len(counters{}))
}

// Output:
// 7
