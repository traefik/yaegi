package main

func main() {
	var a any = 2 + 5
	println(a.(int))
}

// Output:
// 7
