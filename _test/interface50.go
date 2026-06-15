package main

func main() {
	a := true
	var b any = 5
	println(b.(int))
	b = a == true
	println(b.(bool))
}

// Output:
// 5
// true
