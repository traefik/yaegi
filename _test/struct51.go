package main

import "fmt"

type Node struct {
	Name  string
	Child [2]Node
}

func main() {
	a := Node{Name: "hello"}
	a.Child[0] = Node{Name: "world"}
	fmt.Println(a)
	a.Child[0].Child[0] = Node{Name: "sunshine"}
	fmt.Println(a)
}

// Output:
// {hello [{world [<nil> <nil>]} <nil>]}
// {hello [{world [{sunshine [<nil> <nil>]} <nil>]} <nil>]}
