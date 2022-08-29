package main

import "fmt"

type I interface {
	Name() string
}

type S struct {
	iMap map[string]I
}

func main() {
	s := S{}
	s.iMap = map[string]I{}
	fmt.Println(s)
}

// Output:
// {map[]}
