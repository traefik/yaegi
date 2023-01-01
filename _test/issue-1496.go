package main

func main() {
	a := []byte{} == nil
	println(a)
	b := nil == []byte{}
	println(b)
	c := nil == &struct{}{}
	println(c)
	var i int
	i = 100
	d := nil == &i
	println(d)
}

// Output:
// --
// false
// false
// false
// false
