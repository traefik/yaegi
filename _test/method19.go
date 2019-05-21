package main

import "fmt"

func f() (string, error) {
	err := fmt.Errorf("a nice error")
	return "", err
}

func main() {
	_, err := f()
	if err != nil {
		fmt.Println(err.Error())
	}
}

// Output:
// a nice error
