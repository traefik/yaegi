package main

import "fmt"

type Root struct {
	Name string
}

func (r *Root) Hello() {
	fmt.Println("Hello", r.Name)
}

type One = Root

func main() {
	one := &One{Name: "one"}
	displayOne(one)
	displayRoot(one)

	root := &Root{Name: "root"}
	displayOne(root)
	displayRoot(root)
}

func displayOne(val *One) {
	fmt.Println(val)
}

func displayRoot(val *Root) {
	fmt.Println(val)
}

// Output:
// &{one}
// &{one}
// &{root}
// &{root}
