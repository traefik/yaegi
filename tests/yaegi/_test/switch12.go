package main

func main() {
	var i interface{}

	switch a := i.(type) {
	case string:
		println("string", a+" ok")
	case nil:
		println("i is nil")
	default:
		println("unknown")
	}
}

// Output:
// i is nil
