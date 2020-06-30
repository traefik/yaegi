package main

import "compress/flate"

func f1(i int) { println("i:", i) }

func main() {
	i := flate.BestSpeed
	f1(i)
}

// Output:
// i: 1
