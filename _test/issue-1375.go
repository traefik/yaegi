package main

import "fmt"

type Option func(*Struct)

func WithOption(opt string) Option {
	return func(s *Struct) {
		s.opt = opt
	}
}

type Struct struct {
	opt string
}

func New(opts ...Option) *Struct {
	s := new(Struct)
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Struct) ShowOption() {
	fmt.Println(s.opt)
}

func main() {
	opts := []Option{
		WithOption("test"),
	}
	s := New(opts...)
	s.ShowOption()
}

// Output:
// test
