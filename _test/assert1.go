package main

import (
	"fmt"
	"time"
)

type TestStruct struct{}

func (t TestStruct) String() string {
	return "hello world"
}

func main() {
	var t interface{}
	t = time.Nanosecond
	s, ok := t.(fmt.Stringer)
	if !ok {
		fmt.Println("time.Nanosecond does not implement fmt.Stringer")
		return
	}
	fmt.Println(s.String())

	var tt interface{}
	tt = TestStruct{}
	ss, ok := tt.(fmt.Stringer)
	if !ok {
		fmt.Println("TestStuct does not implement fmt.Stringer")
		return
	}
	fmt.Println(ss.String())
}

// Output:
// 1ns
// hello world
