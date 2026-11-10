package main

func main() {
	var a any = 1 < 2
	println(a.(bool))
}

// Output:
// true
