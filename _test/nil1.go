package main

func main() {
	var a error = nil

	if a == nil || a.Error() == "nil" {
		println("a is nil")
	}
}

// Output:
// a is nil
