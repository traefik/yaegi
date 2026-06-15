package main

func main() {
	var i any = "truc"

	switch b := 2; a := i.(type) {
	case string:
		println("string", a+" ok")
	default:
		println("unknown", b)
	}
}

// Output:
// string truc ok
