package main

import (
	"fmt"
	"log"
	"net/http"
)

var v string = "v1.0"

type Middleware struct {
	Name string
}

func (m *Middleware) Handler(w http.ResponseWriter, r *http.Request) {
	log.Println(r.Header.Get("User-Agent"))
	w.Header().Set("test", "ok")
	log.Println(w.Header())
	fmt.Fprintln(w, "Welcome to my website", m.Name)
}

func main() {
	m := &Middleware{"Test"}
	http.HandleFunc("/", m.Handler)
	http.ListenAndServe(":8080", nil)
}
