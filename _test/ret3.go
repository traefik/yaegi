package main

import "fmt"

func r2() (int, int) { return 1, 2 }

func main() {
	fmt.Println(r2())
}

// Output:
// 1 2
