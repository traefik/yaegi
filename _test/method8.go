package main

type Sample struct {
	Name string
	Foo  []string
}

func (s *Sample) foo(j int) {
	for i, v := range s.Foo {
		println(i, v)
	}
}

var samples = []Sample{
	Sample{"hello", []string{"world"}},
}

func main() {
	samples[0].foo(3)
}

// Output:
// 0 world
