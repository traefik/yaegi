package main

type Option interface {
	apply(*T)
}

type T struct {
	s string
}

type opt struct {
	name string
}

func (o *opt) apply(t *T) {
	println(o.name)
}

func BuildOptions() []Option {
	return []Option{
		&opt{"opt1"},
		&opt{"opt2"},
	}
}

func NewT(name string, options ...Option) *T {
	t := &T{name}
	for _, opt := range options {
		opt.apply(t)
	}
	return t
}

func main() {
	t := NewT("hello", BuildOptions()...)
	println(t.s)
}

// Output:
// opt1
// opt2
// hello
