package main

import "fmt"

type Set[Elem comparable] struct {
	m map[Elem]struct{}
}

func Make[Elem comparable]() Set[Elem] {
	return Set[Elem]{m: make(map[Elem]struct{})}
}

func (s Set[Elem]) Add(v Elem) {
	s.m[v] = struct{}{}
}

func main() {
	s := Make[int]()
	s.Add(1)
	fmt.Println(s)
}

// Output:
// {map[1:{}]}
