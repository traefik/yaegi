package main

import "fmt"

func main() {
	str := "part1"
	str += fmt.Sprintf("%s", "part2")
	fmt.Println(str)
}

// Output:
// part1part2
