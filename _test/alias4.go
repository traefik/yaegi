package main

import (
	"fmt"
	"net/http"
)

type A http.Header

func (a A) Test1() {
	fmt.Println("test1")
}

type B A

func (b B) Test2() {
	fmt.Println("test2")
}

func main() {
	b := B{}

	b.Test2()
}

// Output:
// test2
