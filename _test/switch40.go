package main

func f(params ...interface{}) {
	switch params[0].(type) {
	case string:
		println("a string")
	default:
		println("not a string")
	}
}

func main() {
	f("Hello")
}

// Output:
// a string
