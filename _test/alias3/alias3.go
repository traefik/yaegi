package alias3

type T struct {
	A string
}

func (t *T) Print() {
	println(t.A)
}

// Output:
// test
