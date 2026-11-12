package main

type T struct {
	name string
}

var tab = []*T{{
	name: "foo",
}, {
	name: "bar",
}}

func main() {
	println(len(tab))
	println(tab[0].name)
}

// Output:
// 2
// foo
