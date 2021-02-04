package main

import (
	"fmt"
	"time"
)

type MyWriter interface {
	Write(p []byte) (i int, err error)
}

type DummyWriter interface {
	Write(p []byte) (i int, err error)
}

type TestStruct struct{}

func (t TestStruct) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func usesWriter(w MyWriter) {
	n, _ := w.Write([]byte("hello world"))
	fmt.Println(n)
}

type MyStringer interface {
	String() string
}

type DummyStringer interface {
	String() string
}

func usesStringer(s MyStringer) {
	fmt.Println(s.String())
}

func main() {
	// TODO(mpl): restore when we can deal with empty interface.
//	var t interface{}
	var t DummyWriter
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
	n, _ := t.(MyWriter).Write([]byte("hello world"))
	fmt.Println(n)

	// not redundant with the above, because it goes through a slightly different code path.
	if _, ok := t.(MyWriter); !ok {
		fmt.Println("TestStruct does not implement MyWriter")
		return
	} else {
		fmt.Println("TestStruct implements MyWriter")
	}

	// TODO(mpl): restore
	/*
	t = 42
	foo, ok := t.(MyWriter)
	if !ok {
		fmt.Println("42 does not implement MyWriter")
	} else {
		fmt.Println("42 implements MyWriter")
	}
	_ = foo

	if _, ok := t.(MyWriter); !ok {
		fmt.Println("42 does not implement MyWriter")
	} else {
		fmt.Println("42 implements MyWriter")
	}
	*/

	// var tt interface{}
	var tt DummyStringer
	tt = time.Nanosecond
	var myD MyStringer
	myD, ok = tt.(MyStringer)
	if !ok {
		fmt.Println("time.Nanosecond does not implement MyStringer")
	} else {
		fmt.Println("time.Nanosecond implements MyStringer")
		usesStringer(myD)
	}
	fmt.Println(tt.(MyStringer).String())

	if _, ok := tt.(MyStringer); !ok {
		fmt.Println("time.Nanosecond does not implement MyStringer")
	} else {
		fmt.Println("time.Nanosecond implements MyStringer")
	}

	// TODO(mpl): restore
	/*
	tt = 42
	bar, ok := tt.(MyStringer)
	if !ok {
		fmt.Println("42 does not implement MyStringer")
	} else {
		fmt.Println("42 implements MyStringer")
	}
	_ = bar

	if _, ok := tt.(MyStringer); !ok {
		fmt.Println("42 does not implement MyStringer")
	} else {
		fmt.Println("42 implements MyStringer")
	}
	*/
}

// Output:
// TestStruct implements MyWriter
// 11
// 11
// TestStruct implements MyWriter
// time.Nanosecond implements MyStringer
// 1ns
// 1ns
// time.Nanosecond implements MyStringer
