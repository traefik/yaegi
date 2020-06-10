package main

type Root struct {
	Name string
}

type One struct {
	Root
}

type Hi interface {
	Hello() string
}

func (r *Root) Hello() string { return "Hello " + r.Name }

func main() {
	var one interface{} = &One{Root{Name: "test2"}}
	println(one.(Hi).Hello())
}

// Output:
// Hello test2
