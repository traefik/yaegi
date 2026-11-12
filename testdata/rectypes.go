package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Recursive types and interfaces with methods given to native code.

type Tree struct {
	Name     string  `json:"name"`
	Children []*Tree `json:"children,omitempty"`
	Parent   *Tree   `json:"-"`
}

type List struct {
	V    int
	Next *List
}

type Namer interface{ Name() string }

type Reg struct {
	byName map[string]Namer
	n      int
}

type A struct {
	B
}

type B struct {
	*A
	X int
}

func main() {
	root := &Tree{Name: "root"}
	root.Children = append(root.Children, &Tree{Name: "a", Parent: root}, &Tree{Name: "b"})
	root.Children[0].Children = []*Tree{{Name: "a1"}}
	json.NewEncoder(os.Stdout).Encode(root)

	l := &List{1, &List{2, &List{3, nil}}}
	fmt.Println(l.V, l.Next.V, l.Next.Next.V, l.Next.Next.Next == nil)
	fmt.Printf("%v %+v\n", List{}, List{V: 4})

	fmt.Println(Reg{byName: map[string]Namer{}})
	fmt.Println(A{}, B{X: 2})

	var t Tree
	err := json.NewDecoder(strings.NewReader(`{"name":"x"}`)).Decode(&t)
	fmt.Println(t.Name, t.Children == nil, err)

	// Interpreted code keeps the identity of nodes passed to native code.
	fmt.Sprint(root)
	fmt.Println(root.Children[0].Parent == root)

	// Write-back of a pointer argument keeps the identity of nested pointers.
	a := root.Children[0]
	json.Unmarshal([]byte(`{"name":"root2"}`), root)
	fmt.Println(root.Name, root.Children[0] == a, a.Parent == root)
}
