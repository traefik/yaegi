package main

import "fmt"

func main() {
	for i := 0; i < 2; i++ {
		var buf [8]byte
		var x int
		fmt.Println(buf, x)
		for i := range buf {
			buf[i] = byte(i)
			x = i
		}
		fmt.Println(buf, x)
	}
}

// Output:
// [0 0 0 0 0 0 0 0] 0
// [0 1 2 3 4 5 6 7] 7
// [0 0 0 0 0 0 0 0] 0
// [0 1 2 3 4 5 6 7] 7
