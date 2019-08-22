package main

type T1 struct {
	T2 *T2
}

func (t *T1) Get() string {
	return t.T2.V().Name
}

type T2 struct {
	Name string
}

func (t *T2) V() *T2 {
	if t == nil {
		return defaultT2
	}
	return t
}

var defaultT2 = &T2{"no name"}

func main() {
	t := &T1{}
	println(t.Get())
}

// Output:
// no name
