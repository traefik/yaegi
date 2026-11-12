package main

import "fmt"

func main() {
	var fs []func() int
	for i := 0; i < 3; i++ {
		fs = append(fs, func() int { return i })
	}
	for _, v := range []int{7, 8} {
		fs = append(fs, func() int { return v })
	}
	for _, f := range fs {
		fmt.Print(f(), " ")
	}
	fmt.Println()
}
