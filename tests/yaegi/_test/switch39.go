package main

func f(params ...interface{}) {
	switch p0 := params[0].(type) {
	case string:
		println("string:", p0)
	default:
		println("not a string")
	}
}

func main() {
	f("Hello")
}

// Output:
// string: Hello
