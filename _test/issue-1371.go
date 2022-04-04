package main

import "fmt"

type node struct {
	parent *node
	child  []*node
	key    string
}

func main() {
	root := &node{key: "root"}
	root.child = nil
	fmt.Println("root:", root)
}

// Output:
// root: &{<nil> [] root}
