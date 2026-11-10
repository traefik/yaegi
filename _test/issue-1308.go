package main

import "fmt"

type test struct {
	v any
	s string
}

type T struct {
	name string
}

func main() {
	t := []test{
		{
			v: []any{
				T{"hello"},
			},
			s: "world",
		},
	}
	fmt.Println(t)
}

// Output:
// [{[{hello}] world}]
