package main

import (
	"reflect"
	"time"
)

func main() {
	t := time.Date(2009, time.November, 10, 23, 4, 5, 0, time.UTC)
	v := reflect.ValueOf(t.String)
	f := v.Interface().(func() string)
	println(f())
}

// Output:
// 2009-11-10 23:04:05 +0000 UTC
