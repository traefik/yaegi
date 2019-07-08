package main

type S struct {
	Child []*S
	Name  string
}

func main() {
	s := &S{Name: "root"}
	s.Child = append(s.Child, &S{Name: "child"})
	println(s.Child[0].Name)
}

// Output:
// child
