package plugin

import (
	"fmt"
	"net/http"
)

var version = "v1"

type Sample struct{ Name string }

func (s *Sample) Handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Welcome to my website", s.Name, version)
}

func Handler2(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Welcome to my website", version)
}

func NewSample(name string) func(http.ResponseWriter, *http.Request) {
	fmt.Println("in NewSample", name, version)
	s := &Sample{"test"}
	return s.Handler
	//return Handler2
	//return func(w http.ResponseWriter, r *http.Request) { return Handler2(w, r) }
	//return func(w http.ResponseWriter, r *http.Request) { return s.Handler(w, r) }
}

//func main() {
//	s := &Sample{"Test"}
//	http.HandleFunc("/", s.Handler)
//	http.ListenAndServe(":8080", nil)
//}
