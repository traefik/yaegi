package main

import (
	"fmt"
	"time"
)

func test(time string, t time.Time) string {
	return time
}

func main() {
	str := test("test", time.Now())
	fmt.Println(str)
}

// Output:
// test
