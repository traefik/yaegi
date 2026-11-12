package main

import "fmt"

func main() {
	var buf [8]byte
	for i := 0; i < 2; i++ {
		for i := range buf {
			buf[i] += byte(i)
		}
		fmt.Println(buf)
	}
}

// Output:
// [0 1 2 3 4 5 6 7]
// [0 2 4 6 8 10 12 14]
