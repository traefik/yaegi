package main

var optionsG map[string]string = nil

var roundG = 30

func main() {
	dummy := roundG
	roundG = dummy + 1
	println(roundG)
	println(optionsG == nil)
}

// Output:
// 31
// true
