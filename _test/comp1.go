package main

func main() {
	var a interface{} = 1 < 2
	println(a.(bool))
}

// Output:
// true
