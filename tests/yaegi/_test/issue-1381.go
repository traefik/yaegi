package main

import (
	"bytes"
	"fmt"
)

func main() {
	var bufPtrOne *bytes.Buffer
	var bufPtrTwo *bytes.Buffer
	var bufPtrThree *bytes.Buffer
	var bufPtrFour *bytes.Buffer

	for i := 0; i < 2; i++ {
		bufOne := bytes.Buffer{}
		bufTwo := &bytes.Buffer{}
		var bufThree bytes.Buffer
		bufFour := new(bytes.Buffer)

		if bufPtrOne == nil {
			bufPtrOne = &bufOne
		} else if bufPtrOne == &bufOne {
			fmt.Println("bufOne was not properly redeclared")
		} else {
			fmt.Println("bufOne is properly redeclared")
		}

		if bufPtrTwo == nil {
			bufPtrTwo = bufTwo
		} else if bufPtrTwo == bufTwo {
			fmt.Println("bufTwo was not properly redeclared")
		} else {
			fmt.Println("bufTwo is properly redeclared")
		}

		if bufPtrThree == nil {
			bufPtrThree = &bufThree
		} else if bufPtrThree == &bufThree {
			fmt.Println("bufThree was not properly redeclared")
		} else {
			fmt.Println("bufThree is properly redeclared")
		}

		if bufPtrFour == nil {
			bufPtrFour = bufFour
		} else if bufPtrFour == bufFour {
			fmt.Println("bufFour was not properly redeclared")
		} else {
			fmt.Println("bufFour is properly redeclared")
		}
	}
}

// Output:
// bufOne is properly redeclared
// bufTwo is properly redeclared
// bufThree is properly redeclared
// bufFour is properly redeclared
