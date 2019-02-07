package main

import (
	"net/http"
)

func main() {
	http.DefaultServeMux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {})
	http.DefaultServeMux = &http.ServeMux{}
	http.DefaultServeMux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {})
	http.DefaultServeMux = &http.ServeMux{}
}
