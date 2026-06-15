package main

var cc any = 2
var dd = cc.(int)

func main() {
	println(dd)
}

// Output:
// 2
