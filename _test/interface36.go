package main

import "fmt"

var (
	t *S
	_ I = t
	_ J = t
)

type S struct {
	Name string
}

func (s *S) F() int { return len(s.Name) }
func (s *S) G() int { return s.F() }
func (s *S) Ri() I  { return s }
func (s *S) Rj() J  { return s }

type J interface {
	I
	G() int
	Rj() J
}

type I interface {
	F() int
	Ri() I
}

func main() {
	var j J
	fmt.Println(j)
}

// Output:
// <nil>
