package main

type Transformer interface {
	Reset()
}

type Encoder struct {
	Transformer
}

type nop struct{}

func (nop) Reset() { println("Reset") }

func f(e Transformer) {
	e.Reset()
}

func main() {
	e := Encoder{Transformer: nop{}}
	f(e)
}

// Output:
// Reset
