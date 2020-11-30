package main

import (
	"fmt"
	"time"
)

type MyWriter interface {
	Write(p []byte) (i int, err error)
}

type TestStruct struct{}

func (t TestStruct) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func usesWriter(w MyWriter) {
	w.Write(nil)
}

type MyStringer interface {
	String() string
}

func usesStringer(s MyStringer) {
	fmt.Println(s.String())
}

func main() {
	var t interface{}
	t = TestStruct{}
	var tw MyWriter
	var ok bool
	tw, ok = t.(MyWriter)
	if !ok {
		fmt.Println("TestStruct does not implement MyWriter")
	} else {
		fmt.Println("TestStruct implements MyWriter")
		usesWriter(tw)
	}

	var tt interface{}
	tt = time.Nanosecond
	var myD MyStringer
	myD, ok = tt.(MyStringer)
	if !ok {
		fmt.Println("time.Nanosecond does not implement MyStringer")
	} else {
		fmt.Println("time.Nanosecond implements MyStringer")
		usesStringer(myD)
	}
}

// Output:
// TestStruct implements MyWriter
// time.Nanosecond implements MyStringer
// 1ns
