package baz

func NewT() *T {
	return &T{
		A1: make([]U, 0),
		A3: "foobar",
	}
}

type T struct {
	A1 []U
	A3 string
}

type U struct {
	B1 V
	B2 V
}

type V struct {
	C1 string
}
