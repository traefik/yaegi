package main

import (
	"fmt"
	"reflect"
)

// reflect.Copy into a big [n]byte in real bytes (bigByteArray in
// byteview.go) must write the array. The writes went to a decoded copy of
// the array, and were lost.

func main() {
	var a [64]byte
	n := reflect.Copy(reflect.ValueOf(&a).Elem(), reflect.ValueOf([]byte("hi")))
	fmt.Println("from slice:", n, a[0], a[1], a[2])

	n = reflect.Copy(reflect.ValueOf(&a).Elem(), reflect.ValueOf("xyz"))
	fmt.Println("from string:", n, a[:4])

	var b [100]byte
	for i := range b {
		b[i] = byte(i + 1)
	}
	n = reflect.Copy(reflect.ValueOf(&a).Elem(), reflect.ValueOf(&b).Elem())
	fmt.Println("from array:", n, a[0], a[63])

	src := [3]byte{7, 8, 9}
	n = reflect.Copy(reflect.ValueOf(&b).Elem(), reflect.ValueOf(src))
	fmt.Println("from small array:", n, b[:4])

	s := make([]byte, 5)
	n = reflect.Copy(reflect.ValueOf(s), reflect.ValueOf(&b).Elem())
	fmt.Println("to slice:", n, s)
}
