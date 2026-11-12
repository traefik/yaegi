package main

import "fmt"

func main() {
outer:
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if x == 5 && y == 5 {
				break outer
			}
		}
		fmt.Println(y)
	}
	fmt.Println("Yay! I finished!")
}

// Output:
// 0
// 1
// 2
// 3
// 4
// Yay! I finished!
