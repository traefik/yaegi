package main

type Fooer interface {
	Foo() string
}

type Barer interface {
	//fmt.Stringer
	Fooer
	Bar()
}

type T struct{}

func (t *T) Foo() string { return "T: foo" }
func (*T) Bar()          { println("in bar") }

var t = &T{}

func main() {
	var f Barer
	if f != t {
		println("ok")
	}
}

// Output:
// ok
