package plugin

import (
	"fmt"
	"net/http"
)

var version = "v1"

// Sample stores middleware metadata
type Sample struct{ Name string }

// Handler processes requests
func (s *Sample) Handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Welcome to my website", s.Name, version)
}

// Handler2 processes requests
func Handler2(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Welcome to my website", version)
}

// NewSample returns a new sample handler function
func NewSample(name string) func(http.ResponseWriter, *http.Request) {
	s := &Sample{"test"}
	fmt.Println("in NewSample", name, version, s)
	//return s.Handler
	return Handler2
	//return func(w http.ResponseWriter, r *http.Request) { return Handler2(w, r) }
	//return func(w http.ResponseWriter, r *http.Request) { return s.Handler(w, r) }
}

//func main() {
//	s := &Sample{"Test"}
//	http.HandleFunc("/", s.Handler)
//	http.ListenAndServe(":8080", nil)
//}
