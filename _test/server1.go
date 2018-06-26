package main

import (
	"fmt"
	"net/http"
)

type Middleware struct {
	Name string
}

func (m *Middleware) Handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Welcome to my website", m.Name)
}

func main() {
	m := &Middleware{"Test"}
	http.HandleFunc("/", m.Handler)
	http.ListenAndServe(":8080", nil)
}
