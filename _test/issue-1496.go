package main

func main() {
	a := []byte{} == nil
	b := nil == []byte{}
	c := nil == &struct{}{}
	i := 100
	d := nil == &i
	var v interface{}
	f := nil == v
	g := v == nil
	println(a, b, c, d, f, g)
}

// Output:
// false false false false true true
