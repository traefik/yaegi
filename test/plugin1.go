package sample

import (
	"fmt"
	"net/http"
)

type Sample struct{ Name string }

func NewSample(name string) *Sample {
	return &Sample{Name: name}
}

func (s *Sample) Handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Welcome to my website")
}

func WrapHandler(s *Sample, w httpResponseWriter, r *http.Request) {
	s.Handler(w, r)
}

//func main() {
//	m := &Middleware{"Test"}
//	http.HandleFunc("/", Handler)
//	http.ListenAndServe(":8080", nil)
//}
