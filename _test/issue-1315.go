package main

type Intf interface {
	M()
}

type T struct {
	s string
}

func (t *T) M() { println("in M") }

func f(i any) {
	switch j := i.(type) {
	case Intf:
		j.M()
	default:
		println("default")
	}
}

func main() {
	var i Intf
	var k any = 1
	i = &T{"hello"}
	f(i)
	f(k)
}

// Output:
// in M
// default
