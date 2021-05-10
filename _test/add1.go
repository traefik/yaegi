package main

func main() {
	b := 2 // int

	var c int = 5 + b
	println(c)

	var d int32 = 6 + int32(b)
	println(d)

	var a interface{} = 7 + b
	println(a.(int))

	var e int32 = 2
	var f interface{} = 8 + e
	println(f.(int32))

	a = 9 + e
	println(a.(int32))

	var g int = 2
	a = 10 + g
	println(a.(int))

	// multiple assignment
	var foo interface{}
	foo, a = "hello", 11 + g
	println(a.(int))
	println(foo.(string))
}

// Output:
// 7
// 8
// 9
// 10
// 11
// 12
// 13
// hello
