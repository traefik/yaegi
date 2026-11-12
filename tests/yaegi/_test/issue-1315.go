package main

type Intf interface {
	M()
}

type T struct {
	s string
}

func (t *T) M() { println("in M") }

func f(i interface{}) {
	switch j := i.(type) {
	case Intf:
		j.M()
	default:
		println("default")
	}
}

func main() {
	var i Intf
	var k interface{} = 1
	i = &T{"hello"}
	f(i)
	f(k)
}

// Output:
// in M
// default
