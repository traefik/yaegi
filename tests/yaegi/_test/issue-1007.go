package main

type TypeA struct {
	B TypeB
}

type TypeB struct {
	C1 *TypeC
	C2 *TypeC
}

type TypeC struct {
	Val string
	D   *TypeD
	D2  *TypeD
}

type TypeD struct {
	Name string
}

func build() *TypeA {
	return &TypeA{
		B: TypeB{
			C2: &TypeC{Val: "22"},
		},
	}
}

func Bar(s string) string {
	a := build()
	return s + "-" + a.B.C2.Val
}

func main() {
	println(Bar("test"))
}

// Output:
// test-22
