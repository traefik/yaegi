package main

import (
	"fmt"
	"net/http"
)

var myHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello world"))
})

type T1 struct {
	Name string
}

func (t *T1) Handler(h http.Handler) http.Handler {
	fmt.Println("#1", t.Name)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("#2", t.Name)
		h.ServeHTTP(w, r)
	})
}

func main() {
	t := &T1{"myName"}
	handler := t.Handler(myHandler)
	http.ListenAndServe(":8080", handler)
}
