package main

func main() {
	b := 2
	var a interface{} = 5 + b
	println(a.(int))
}

// Output:
// 7
