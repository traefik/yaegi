package main

import (
	"fmt"
	"io"
)

func main() {
	x := io.LimitedReader{}
	y := io.Reader(&x)
	fmt.Println(y)
}
