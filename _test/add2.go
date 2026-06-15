package main

type iface any

func main() {
	b := 2
	var a iface = 5 + b
	println(a.(int))
}

// Output:
// 7
