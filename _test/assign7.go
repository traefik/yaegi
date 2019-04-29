package main

import "fmt"

func main() {
	a := 3
	t := map[string]int{"a": 1, "b": 2}
	s := []int{4, 5}
	fmt.Println(a, t, s)
	a, t["b"], s[1] = t["b"], s[1], a
	fmt.Println(a, t, s)
}

// Output:
// 3 map[a:1 b:2] [4 5]
// 2 map[a:1 b:5] [4 3]
