package main

var a = [...]bool{true, true}

func main() {
	println(a[0] && true)
}

// Output:
// true
