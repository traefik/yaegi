package main

import "fmt"

func Count(n int) func(func(int) bool) {
	return func(yield func(int) bool) {
		for i := 0; i < n; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

func main() {
	for i := range Count(5) {
		if i == 3 {
			break
		}
		fmt.Print(i, " ")
	}
	fmt.Println()
}
