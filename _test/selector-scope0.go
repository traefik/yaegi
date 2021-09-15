package main

import (
	"fmt"
	"time"
)

func test(time string, t time.Time) string {
	return time
}

var zero = time.Time{}

func test2(time string) time.Time {
	return zero
}

func main() {
	str := test("test", time.Now())
	fmt.Println(str)

	str2 := test2("test2")
	fmt.Println(str2)
}

// Output:
// test
// 0001-01-01 00:00:00 +0000 UTC
