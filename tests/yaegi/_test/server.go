package main

import (
	"fmt"
	"net/http"
)

var v string = "v1.0"

func main() {
	a := "hello "
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Welcome to my website! ", a, v)
	})

	http.ListenAndServe(":8080", nil)
}
