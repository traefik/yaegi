package main

var m = map[string]int{"foo": 1, "bar": 2}

func main() {
	var a any = m["foo"]
	println(a.(int))
}

// Output:
// 1
