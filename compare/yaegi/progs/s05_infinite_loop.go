package main

import "fmt"

func main() {
	n := 0
	for {
		n++
		if n < 0 {
			break
		}
	}
	fmt.Println(n)
}
