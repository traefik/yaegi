package main

func main() {
	var i interface{}

	switch a := i.(type) {
	case string:
		println("string", a+" ok")
	case i:
		println("i is dummy")
	default:
		println("unknown")
	}
}

// Error:
// 9:2: i is not a type
