package main

import "fmt"

func main() {
	fmt.Println(f())
}

func f() (int, int) { return 2, 3 }

// Output:
// 2 3
