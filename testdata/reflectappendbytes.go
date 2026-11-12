package main

// reflect.Append and reflect.AppendSlice of a []byte, when the elements do
// not all fit in the free capacity. Go appends them all at once: it makes a
// new array and writes nothing into the old one, which another slice may
// share. The VM reflect appended the elements one by one, so the first ones
// went into the shared array before the new array was made.

import (
	"fmt"
	"reflect"
)

func main() {
	// Not all fit (2 free, 3 elements): the shared array does not change.
	b := make([]byte, 14, 16)
	r := reflect.Append(reflect.ValueOf(b), reflect.ValueOf(byte(7)), reflect.ValueOf(byte(8)), reflect.ValueOf(byte(9)))
	fmt.Println(b[:16], r.Len(), r.Bytes()[14:])

	s := make([]byte, 3, 4)
	r = reflect.AppendSlice(reflect.ValueOf(s), reflect.ValueOf([]byte{1, 2}))
	fmt.Println(s[:4], r.Len(), r.Bytes())

	// All fit: the elements go into the shared array, as in Go.
	f := make([]byte, 2, 4)
	r = reflect.Append(reflect.ValueOf(f), reflect.ValueOf(byte(5)), reflect.ValueOf(byte(6)))
	fmt.Println(f[:4], r.Len(), r.Bytes())

	// A named byte slice type.
	type buf []byte
	n := make(buf, 1, 2)
	r = reflect.Append(reflect.ValueOf(n), reflect.ValueOf(byte(3)), reflect.ValueOf(byte(4)))
	fmt.Println(n[:2], r.Len(), r.Interface().(buf))
}
