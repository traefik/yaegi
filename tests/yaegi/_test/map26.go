package main

var m = map[string]int{"foo": 1, "bar": 2}

func main() {
	var a interface{} = m["foo"]
	println(a.(int))
}

// Output:
// 1
