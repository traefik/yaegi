package main

type myInterface interface {
	myFunc() string
}

type V struct{}

func (v *V) myFunc() string { return "hello" }

type U struct {
	v myInterface
}

func (u *U) myFunc() string { return u.v.myFunc() }

func main() {
	x := V{}
	y := myInterface(&x)
	y = &U{y}
	println(y.myFunc())
}

// Output:
// hello
