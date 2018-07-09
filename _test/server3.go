package main

import (
	"net/http"
)

//var myHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hello world")) })
//var myHandler = func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hello world")) }
func myHandler(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hello world")) }

func main() {
	http.HandleFunc("/", myHandler)
	http.ListenAndServe(":8080", nil)
}
