package main

import "cmp"

func main() {
	println(cmp.Compare(3, 2))
	println(cmp.Less(3, 2))
}

// Output:
// 1
// false
