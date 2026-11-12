package main

func main() {
	type A struct{ *A }
	v := &A{}
	v.A = v
	println("v.A.A = v", v.A.A == v)
}

// Output:
// v.A.A = v true
