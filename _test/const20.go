package main

import "fmt"

const maxLen = int64(int(^uint(0) >> 1))

func main() {
	fmt.Println(maxLen)
}

// Output:
// 9223372036854775807
