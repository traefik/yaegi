package main

func main() {
	var b interface{} = !(1 == 2)
	println(b.(bool))
}

// Output:
// true
