package main

import "fmt"

func check() (result bool, err error) {
	return true, nil
}

func main() {
	result, error := check()
	fmt.Println(result, error)
}

// Output:
// true <nil>
