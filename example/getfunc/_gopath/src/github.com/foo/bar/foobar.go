package bar

type Foo struct {
	A string
}

func NewFoo() *Foo {
	return &Foo{A: "test"}
}
