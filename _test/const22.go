package main

const (
	numDec uint8 = (1 << iota) / 2
	numHex
	numOct
	numFloat
)

func main() {
	println(13 & (numHex | numOct))
}

// Output:
// 1
