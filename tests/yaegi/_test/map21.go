package main

var m = map[int]string{
	1: "foo",
}

func main() {
	var ok bool
	if _, ok = m[1]; ok {
		println("ok")
	}
}

// Output:
// ok
