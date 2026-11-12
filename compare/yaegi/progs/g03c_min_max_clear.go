package main

import "fmt"

func main() {
	fmt.Println(min(3, 1, 2), max(2.5, 1), min("b", "a"))
	m := map[string]int{"a": 1}
	s := []int{1, 2}
	clear(m)
	clear(s)
	fmt.Println(len(m), s)
}
