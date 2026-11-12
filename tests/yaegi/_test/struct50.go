package main

import "fmt"

type Node struct {
	Name  string
	Child []Node
}

func main() {
	a := Node{Name: "hello"}
	a.Child = append([]Node{}, Node{Name: "world"})
	fmt.Println(a)
	a.Child[0].Child = append([]Node{}, Node{Name: "sunshine"})
	fmt.Println(a)
}

// Output:
// {hello [{world []}]}
// {hello [{world [{sunshine []}]}]}
