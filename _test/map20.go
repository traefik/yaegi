package main

var a = map[int]bool{1: true, 2: true}

func main() {
	println(a[1] && true)
}

// Output:
// true
