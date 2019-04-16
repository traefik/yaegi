package main

import (
	"fmt"
	"io/ioutil"
	"os"
)

func main() {
	file, err := ioutil.TempFile("", "yeagibench")
	if err != nil {
		panic(err)
	}

	n, err := file.Write([]byte("hello world"))
	if err != nil {
		panic(err)
	}
	fmt.Println("n:", n)

	err = file.Close()
	if err != nil {
		panic(err)
	}

	b, err := ioutil.ReadFile(file.Name())
	if err != nil {
		panic(err)
	}
	fmt.Println("b:", string(b))

	err = os.Remove(file.Name())
	if err != nil {
		panic(err)
	}
}

// Output:
// n: 11
// b: hello world
