package main

import (
	"errors"
)

func ShortVariableDeclarations() (i int, err error) {
	r, err := 1, errors.New("test")
	i = r
	return
}

func main() {
	i, er := ShortVariableDeclarations()
	if er != nil {
		println("ShortVariableDeclarations ok")
	} else {
		println("ShortVariableDeclarations not ok")
	}
}

// Output:
// ShortVariableDeclarations ok
