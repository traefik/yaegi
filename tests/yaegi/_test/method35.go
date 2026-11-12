package main

import "strconv"

func main() {
	var err error
	_, err = strconv.Atoi("erwer")
	if _, ok := err.(*strconv.NumError); ok {
		println("here")
	}
}

// Output:
// here
