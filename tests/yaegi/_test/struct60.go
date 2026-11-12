package main

import (
	"fmt"
)

type data struct {
	S string
}

func render(v interface{}) {
	fmt.Println(v)
}

func main() {
	render(data{})
}

// Output:
// {}
