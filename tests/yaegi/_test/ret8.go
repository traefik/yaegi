package main

import "fmt"

type CustomError string

func (s CustomError) Error() string {
	return string(s)
}

func NewCustomError(errorText string) CustomError {
	return CustomError(errorText)
}

func fail() (err error) {
	return NewCustomError("Everything is going wrong!")
}

func main() {
	fmt.Println(fail())
	var myError error
	myError = NewCustomError("ok")
	fmt.Println(myError)
}

// Output:
// Everything is going wrong!
// ok
