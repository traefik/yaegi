package main

import "fmt"

func f(i int) bool {
	switch i {
	case 0:
		println(i)
		return false
	default:
		println("not nul")
		return true
	}
}

func main() {
	r0 := f(0)
	fmt.Printf("%T %v", r0, r0)
	fmt.Println()
	r1 := f(1)
	fmt.Printf("%T %v", r1, r1)
	fmt.Println()
}

// Output:
// 0
// bool false
// not nul
// bool true
