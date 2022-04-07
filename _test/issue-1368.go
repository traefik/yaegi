package main

const dollar byte = 36

func main() {
	var c byte = 36
	switch true {
	case c == dollar:
		println("ok")
	default:
		println("not ok")
	}
}

// Output:
// ok
