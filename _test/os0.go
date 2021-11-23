package main

import (
	"fmt"
	"os"
)

func main() {
	_, err := os.ReadFile("__NotExisting__")
	if err != nil {
		fmt.Println(err.Error())
	}
}

// Output:
// open __NotExisting__: no such file or directory
