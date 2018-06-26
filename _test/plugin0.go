package sample

import (
	"fmt"
	"net/http"
)

var version = "test"

func Handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Welcome to my website", version)
}

//func main() {
//	m := &Middleware{"Test"}
//	http.HandleFunc("/", Handler)
//	http.ListenAndServe(":8080", nil)
//}
