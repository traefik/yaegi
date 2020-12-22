package main

func main() {
	b := 2 // int

	var c int = 5 + b
	println(c)

	var d int32 = 5 + int32(b)
	println(d)

	var a interface{} = 5 + b
	println(a.(int))

	var e int32 = 2
	var f interface{} = 5 + e
	println(f.(int32))

	// TODO(mpl): make this work
//	a = 5 + e
//	println(a.(int32))
}

// Output:
// 7
// 7
// 7
// 7

