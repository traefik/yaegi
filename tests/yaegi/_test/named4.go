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

func (b B) Test3() {
	for k, vals := range b {
		for _, v := range vals {
			fmt.Println(k, v)
		}
	}
}

func main() {
	b := B{}

	b.Test2()
	b["test"] = []string{"a", "b"}
	b.Test3()
}

// Output:
// test2
// test a
// test b
