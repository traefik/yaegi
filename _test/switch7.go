package main

func main() {
	var i any = "truc"

	switch i.(type) {
	case string:
		println("string")
	default:
		println("unknown")
	}
}

// Output:
// string
