package sample

type Sample struct{ Name string }

func (s *Sample) Test() {
	println("Hello from test", s.Name)
}

// Output:
//
