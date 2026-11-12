package main

import (
	"errors"
	"fmt"
	"strings"
)

func main() {
	err := errors.New("hello there")

	switch true {
	case err == nil:
		break
	case strings.Contains(err.Error(), "hello"):
		fmt.Println("True!")
	default:
		fmt.Println("False!")
	}
}

// Output:
// True!
