package main

import "fmt"

type foobar struct {
	callback func(string) func()
}

func cb(text string) func() {
	return func() {
		fmt.Println(text)
	}
}

func main() {
	// These ways of invoking it all work...
	cb("Hi from inline callback!")()

	asVarTest1 := cb("Hi from asVarTest1 callback!")
	asVarTest1()

	asVarTest2 := cb
	asVarTest2("Hi from asVarTest2 callback!")()

	// But inside a struct panics in yaegi...
	asStructField := &foobar{callback: cb}
	asStructField.callback("Hi from struct field callback!")() // <--- panics here
}

// Output:
// Hi from inline callback!
// Hi from asVarTest1 callback!
// Hi from asVarTest2 callback!
// Hi from struct field callback!
