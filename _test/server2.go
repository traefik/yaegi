package main

import (
	"fmt"
	"net/http"
)

var v string = "v1.0"

func main() {

	myHandler := func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Welcome to my website!")
	}

	http.HandleFunc("/", myHandler)
	http.ListenAndServe(":8080", nil)
}
