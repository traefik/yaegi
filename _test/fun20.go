package main

import "fmt"

var myerr error = fmt.Errorf("bar")

func ferr() error { return myerr }

func foo() ([]string, error) {
	return nil, ferr()
}

func main() {
	a, b := foo()
	fmt.Println(a, b)
}

// Output:
// [] bar
