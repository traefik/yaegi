package main

import (
	"fmt"
	"strconv"
)

func main() {
	var value interface{}
	var err error
	value, err = strconv.ParseFloat("123", 64)
	fmt.Println(value, err)
}

// Output:
// 123 <nil>
