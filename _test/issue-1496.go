package main

func main() {
	a := []byte{} == nil
	b := nil == []byte{}
	c := nil == &struct{}{}
	i := 100
	d := nil == &i
	println(a, b, c, d)
}

// Output:
// --
// false false false false
