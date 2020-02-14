package main

type T struct {
	bool
}

var t = T{true}

func main() {
	println(t.bool && true)
}

// Output:
// true
