package main

type Option interface {
	apply()
}

func f(opts ...Option) {
	for _, opt := range opts {
		opt.apply()
	}
}

type T struct{}

func (t *T) apply() { println("in apply") }

func main() {
	opt := []Option{&T{}}
	f(opt[0]) // works
	f(opt...) // fails
}

// Output:
// in apply
// in apply
