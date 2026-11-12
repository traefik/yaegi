package main

type S struct {
	t *T
}

func newS() *S {
	return &S{
		t: &T{u: map[string]*U{}},
	}
}

type T struct {
	u map[string]*U
}

type U struct {
	a int
}

func main() {
	s := newS()
	_ = s

	println("ok")
}
