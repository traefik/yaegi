package main

type S struct {
	Child *S
	Name  string
}

func main() {
	s := &S{Name: "root"}
	s.Child = &S{Name: "child"}
	println(s.Child.Name)
}

// Output:
// child
