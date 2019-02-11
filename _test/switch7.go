package main

func main() {
	var i interface{} = "truc"

	switch i.(type) {
	case string:
		println("string")
	default:
		println("unknown")
	}
}

// Output:
// string
