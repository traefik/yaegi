package main

import "fmt"

type Error interface {
	error
	Code() string
}

type MyError Error

type T struct {
	Name string
}

func (t *T) Error() string { return "err: " + t.Name }
func (t *T) Code() string  { return "code: " + t.Name }

func newT(s string) MyError { return &T{s} }

func main() {
	t := newT("foo")
	fmt.Println(t.Code())
}

// Output:
// code: foo
