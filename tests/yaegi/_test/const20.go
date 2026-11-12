package main

import "fmt"

const maxLen = int64(int64(^uint64(0) >> 1))

func main() {
	fmt.Println(maxLen)
}

// Output:
// 9223372036854775807
