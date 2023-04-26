package main

var cc interface{} = 2
var dd = cc.(int)

func main() {
	println(dd)
}

// Output:
// 2
