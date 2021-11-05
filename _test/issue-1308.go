package main

import "fmt"

type test struct {
	v interface{}
	s string
}

type T struct {
	name string
}

func main() {
	t := []test{
		{
			v: []interface{}{
				T{"hello"},
			},
			s: "world",
		},
	}
	fmt.Println(t)
}

// Output:
// [{[{hello}] world}]
