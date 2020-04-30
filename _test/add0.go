package main

func main() {
	var a interface{} = 2 + 5
	println(a.(int))
}

// Output:
// 7
