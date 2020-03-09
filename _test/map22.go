package main

var m = map[int]string{
	1: "foo",
}

func main() {
	var s string
	s, _ = m[1]
	println(s)
}

// Output:
// foo
