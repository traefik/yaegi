package main

func main() {
	a := []byte{} == nil
	b := nil == []byte{}
	println(a == false, b == false, a == b)
}

// Output:
// true true true
