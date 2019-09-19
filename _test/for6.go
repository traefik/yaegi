package main

import "fmt"

func main() {
	s := "三"
	for i := 0; i < len(s); i++ {
		fmt.Printf("byte %d: %d\n", i, s[i])
	}
	for i, r := range s {
		fmt.Printf("rune %d: %d\n", i, r)
	}
}
