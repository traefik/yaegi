package main

import "fmt"

func main() {
	fmt.Println(min(3, 1, 2), max(3, 1, 2))
	fmt.Println(min(2.5, 1.5), max("a", "b"))

	m := map[string]int{"a": 1, "b": 2}
	clear(m)
	fmt.Println(len(m))

	s := make([]int, 0, 2)
	for i := 0; i < 5; i++ {
		s = append(s, i)
	}
	fmt.Println(s, len(s))

	dst := make([]int, 3)
	n := copy(dst, s)
	fmt.Println(n, dst)

	// range over integer (Go 1.22)
	sum := 0
	for i := range 5 {
		sum += i
	}
	fmt.Println(sum)

	a := [...]int{10, 20, 30}
	fmt.Println(len(a), a[1])
}
