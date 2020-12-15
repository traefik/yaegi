package main

type Doer interface {
	Do() error
}

type T struct {
	Name string
}

func (t *T) Do() error { println("in do"); return nil }

func f() (Doer, error) { return &T{"truc"}, nil }

type Ev struct {
	doer func() (Doer, error)
}

func (e *Ev) do() {
	d, _ := e.doer()
	d.Do()
}

func main() {
	e := &Ev{f}
	println(e != nil)
	e.do()
}

// Output:
// true
// in do
