package main

import (
	"fmt"
	"io/ioutil"
)

func main() {
	_, err := ioutil.ReadFile("__NotExisting__")
	if err != nil {
		fmt.Println(err.Error())
	}
}

// Output:
// open __NotExisting__: no such file or directory
