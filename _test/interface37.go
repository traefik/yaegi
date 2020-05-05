package main

type I interface {
	A() string
	B() string
}

type s struct{}

func NewS() (I, error) {
	return &s{}, nil
}

func (c *s) A() string { return "a" }
func (c *s) B() string { return "b" }

func main() {
	s, _ := NewS()
	println(s.A())
}

// Output:
// a
