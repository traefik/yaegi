package main

import "strconv"

func main() {
	str := strconv.Itoa(101)
	println(str[0] == '1')
}

// Output:
// true
