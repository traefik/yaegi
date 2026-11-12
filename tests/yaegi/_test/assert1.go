package main

import (
	"fmt"
	"reflect"
	"time"
)

type TestStruct struct{}

func (t TestStruct) String() string {
	return "hello world"
}

type DummyStringer interface{
	String() string
}

func main() {
	aType := reflect.TypeOf((*fmt.Stringer)(nil)).Elem()

	var t interface{}
	t = time.Nanosecond
	s, ok := t.(fmt.Stringer)
	if !ok {
		fmt.Println("time.Nanosecond does not implement fmt.Stringer")
		return
	}
	fmt.Println(s.String())
	fmt.Println(t.(fmt.Stringer).String())
	bType := reflect.TypeOf(time.Nanosecond)
	fmt.Println(bType.Implements(aType))

	// not redundant with the above, because it goes through a slightly different code path.
	if _, ok := t.(fmt.Stringer); !ok {
		fmt.Println("time.Nanosecond does not implement fmt.Stringer")
		return
	} else {
		fmt.Println("time.Nanosecond implements fmt.Stringer")
	}

	t = 42
	foo, ok := t.(fmt.Stringer)
	if !ok {
		fmt.Println("42 does not implement fmt.Stringer")
	} else {
		fmt.Println("42 implements fmt.Stringer")
		return
	}
	_ = foo

	if _, ok := t.(fmt.Stringer); !ok {
		fmt.Println("42 does not implement fmt.Stringer")
	} else {
		fmt.Println("42 implements fmt.Stringer")
		return
	}

	// TODO(mpl): restore when fixed
	// var tt interface{}
	var tt DummyStringer
	tt = TestStruct{}
	ss, ok := tt.(fmt.Stringer)
	if !ok {
		fmt.Println("TestStuct does not implement fmt.Stringer")
		return
	}
	fmt.Println(ss.String())
	fmt.Println(tt.(fmt.Stringer).String())

	if _, ok := tt.(fmt.Stringer); !ok {
		fmt.Println("TestStuct does not implement fmt.Stringer")
		return
	} else {
		fmt.Println("TestStuct implements fmt.Stringer")
	}
}

// Output:
// 1ns
// 1ns
// true
// time.Nanosecond implements fmt.Stringer
// 42 does not implement fmt.Stringer
// 42 does not implement fmt.Stringer
// hello world
// hello world
// TestStuct implements fmt.Stringer
