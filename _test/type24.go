package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

func main() {
	assertInt()
	assertNil()
	assertValue()
}

func assertInt() {
	defer func() {
		r := recover()
		fmt.Println(r)
	}()

	var v interface{} = 1
	println(v.(string))
}

func assertNil() {
	defer func() {
		r := recover()
		fmt.Println(r)
	}()

	var v interface{}
	println(v.(string))
}

func assertValue() {
	defer func() {
		r := recover()
		fmt.Println(r)
	}()

	var v http.ResponseWriter = httptest.NewRecorder()
	println(v.(http.Pusher))
}

// Output:
// 22:10: interface conversion: interface {} is int, not string
// 32:10: interface conversion: interface {} is nil, not string
// 42:10: interface conversion: *httptest.ResponseRecorder is not http.Pusher: missing method Push
