package main

type T struct {
	name string
	size int
}

var table = map[int]*T{
	0: {
		name: "foo",
		size: 2,
	}}

var s = table[0].size

func main() {
	println(s)
}

// Output:
// 2
