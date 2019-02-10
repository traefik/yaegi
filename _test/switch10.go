package main

func main() {
	var i interface{} = "truc"

	switch a := i.(type) {
	case string:
		println("string", a+" ok")
	default:
		println("unknown")
	}
}

// Output:
// string truc ok
