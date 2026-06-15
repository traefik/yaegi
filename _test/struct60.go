package main

import (
	"fmt"
)

type data struct {
	S string
}

func render(v any) {
	fmt.Println(v)
}

func main() {
	render(data{})
}

// Output:
// {}
