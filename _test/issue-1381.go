package main

import (
	"bytes"
	"fmt"
)

func main() {
	var bufPtrOne *bytes.Buffer
	var bufPtrTwo *bytes.Buffer

	for i := 0; i < 2; i++ {
		bufOne := bytes.Buffer{}
		bufTwo := &bytes.Buffer{}

		if bufPtrOne == nil {
			bufPtrOne = &bufOne
		} else if bufPtrOne == &bufOne {
			fmt.Println("KO")
		}
		if bufPtrTwo == nil {
			bufPtrTwo = bufTwo
		} else if bufPtrTwo == bufTwo {
			fmt.Println("KO")
		}
	}

	bufPtrOne = nil
	bufPtrTwo = nil

	for i := 0; i < 2; i++ {
		var bufOne bytes.Buffer
		bufTwo := new(bytes.Buffer)

		if bufPtrOne == nil {
			bufPtrOne = &bufOne
		} else if bufPtrOne != &bufOne {
			fmt.Println("OK")
		}
		if bufPtrTwo == nil {
			bufPtrTwo = bufTwo
		} else if bufPtrTwo != bufTwo {
			fmt.Println("OK")
		}
	}
}

// Output:
// OK
// OK
