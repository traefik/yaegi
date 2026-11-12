package main

import "fmt"

func f() (int, int) { return 2, 3 }

func main() {
	fmt.Println(f())
}

// Output:
// 2 3
