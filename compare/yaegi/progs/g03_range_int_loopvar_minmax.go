package main

import (
	"fmt"
	"sync"
)

func main() {
	for i := range 3 {
		fmt.Print(i)
	}
	fmt.Println()
	var fs []func() int
	for i := 0; i < 3; i++ {
		fs = append(fs, func() int { return i })
	}
	for _, f := range fs {
		fmt.Print(f())
	}
	fmt.Println()
	var wg sync.WaitGroup
	res := make([]int, 3)
	for i, v := range []int{10, 20, 30} {
		wg.Add(1)
		go func() { defer wg.Done(); res[i] = v }()
	}
	wg.Wait()
	fmt.Println(res)
	fmt.Println(min(3, 1, 2), max(2.5, 1), min("b", "a"))
	m := map[string]int{"a": 1}
	s := []int{1, 2}
	clear(m)
	clear(s)
	fmt.Println(len(m), s)
}
