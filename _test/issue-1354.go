package main

func main() {
	println(test()) // Go prints true, Yaegi false
}

func test() bool {
	if true {
		goto label
	}
	goto label
label:
	println("Go continues here")
	return true
	println("Yaegi goes straight to this return (this line is never printed)")
	return false
}

// Output:
// Go continues here
// true
