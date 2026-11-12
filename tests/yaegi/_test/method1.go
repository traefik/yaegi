package main

type Sample struct {
	Name string
}

func (s *Sample) foo(i int) {
	println("in foo", s.Name, i)
}

func main() {
	sample := Sample{"hello"}
	s := &sample
	s.foo(3)
}

// Output:
// in foo hello 3
