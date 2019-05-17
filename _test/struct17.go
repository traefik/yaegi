package main

type S1 struct {
	Name string
}

type S2 struct {
	*S1
}

func main() {
	s1 := &S1{"foo"}
	s2 := S2{s1}
	println(s2.Name)
}

// Output:
// foo
