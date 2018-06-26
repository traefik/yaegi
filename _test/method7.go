package main

type Sample struct {
	Name string
}

func (s *Sample) foo(i int) {
	println("in foo", s.Name, i)
}

var samples = []Sample{
	Sample{"hello"},
}

func main() {
	samples[0].foo(3)
}

// Output:
// in foo hello 3
