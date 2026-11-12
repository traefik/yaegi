package main

type Node struct {
	Name  string
	Alias *Node
	Child []*Node
}

func main() {
	n := &Node{Name: "parent"}
	n.Child = append(n.Child, &Node{Name: "child"})
	println(n.Name, n.Child[0].Name)
}

// Output:
// parent child
