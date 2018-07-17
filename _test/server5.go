package main

import (
	"net/http"
)

var myHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello world"))
})

func main() {
	http.ListenAndServe(":8080", myHandler)
}
