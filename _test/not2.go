package main

func main() {
	var b any = !(1 == 2)
	println(b.(bool))
}

// Output:
// true
