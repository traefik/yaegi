package main

import (
	"encoding/json"
	"os"
)

type Node struct {
	Name  string
	Child [2]*Node
}

func main() {
	a := Node{Name: "hello"}
	a.Child[0] = &Node{Name: "world"}
	json.NewEncoder(os.Stdout).Encode(a)
	a.Child[0].Child[0] = &Node{Name: "sunshine"}
	json.NewEncoder(os.Stdout).Encode(a)
}

// Output:
// {"Name":"hello","Child":[{"Name":"world","Child":[null,null]},null]}
// {"Name":"hello","Child":[{"Name":"world","Child":[{"Name":"sunshine","Child":[null,null]},null]},null]}
