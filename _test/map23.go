package main

var m = map[int]string{
	1: "foo",
}

func main() {
	_, _ = m[1]
	println("ok")
}

// Output:
// ok
