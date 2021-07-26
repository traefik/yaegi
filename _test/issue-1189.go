package main

type I interface {
	Foo() int
}

type S1 struct {
	i int
}

func (s S1) Foo() int { return s.i }

type S2 struct{}

func (s *S2) Foo() int { return 42 }

func main() {
	Is := map[string]I{
		"foo": S1{21},
		"bar": &S2{},
	}
	n := 0
	for _, s := range Is {
		n += s.Foo()
	}
	bar := "bar"
	println(n, Is["foo"].Foo(), Is[bar].Foo())
}

// Output:
// 63 21 42
