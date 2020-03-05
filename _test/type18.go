package main

type T struct {
	name string
	size int
}

var table = []*T{{
	name: "foo",
	size: 2,
}}

var s = table[0].size

func main() {
	println(s)
}

// Output:
// 2
