package main

import "fmt"

type Int int

func (I Int) String() string {
	return "foo"
}

func main() {
	var i Int
	var st fmt.Stringer = i
	fmt.Println(st.String())
}

// Output:
// foo
