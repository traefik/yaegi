package main

type I interface{ Hello() }

type T struct{ Name string }

func (t *T) Hello() { println("Hello", t.Name) }

type FT func(i I)

type ST struct{ Handler FT }

func newF() FT {
	return func(i I) {
		i.Hello()
	}
}

func main() {
	st := &ST{}
	st.Handler = newF()
	st.Handler(&T{"test"})
}

// Output:
// Hello test
