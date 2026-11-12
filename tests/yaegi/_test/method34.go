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

type Hey interface {
	Hello() string
}

func (r *Root) Hello() string { return "Hello " + r.Name }

func main() {
	// TODO(mpl): restore when type assertions work again.
	// var one interface{} = &One{Root{Name: "test2"}}
	var one Hey = &One{Root{Name: "test2"}}
	println(one.(Hi).Hello())
}

// Output:
// Hello test2
