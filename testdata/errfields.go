package main

import "fmt"

// A struct of the program with error fields, exported and not, given to
// fmt: Go calls Error for the exported field only (fmt cannot call a
// method through an unexported field), at the top level and inside []any.

type ev struct{ msg string }

func (e ev) Error() string { return e.msg }

type T struct {
	E error
	e error
}

func main() {
	v := struct {
		E error
		e error
	}{ev{"E"}, ev{"e"}}
	fmt.Printf("%v\n%+v\n", v, v)
	t := T{ev{"E"}, ev{"plain"}}
	fmt.Printf("%v\n%+v\n", t, t)
	fmt.Printf("%v\n%+v\n", []any{v, t}, map[string]any{"t": t})
	fmt.Println(fmt.Errorf("wrap: %v", t))
}
