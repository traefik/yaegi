package main

func main() {
	a := 0
	b := true
	c := false
	if b && c {
		a = 1
	} else {
		a = -1
	}
	println(a)
}

// Output:
// -1
