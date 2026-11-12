package main

type T struct{}

func (t T) Error() string { return "T: error" }

var invalidT = T{}

func main() {
	var err error
	if err != invalidT {
		println("ok")
	}
}

// Output:
// ok
