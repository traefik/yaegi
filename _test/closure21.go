package main

import "fmt"

func main() {
	var fs []func() int
	for i := 0; i < 6; i++ {
		i++
		fs = append(fs, func() int { return i })
	}
	for _, f := range fs {
		fmt.Println(f())
	}
}

// Output:
// 1
// 3
// 5
