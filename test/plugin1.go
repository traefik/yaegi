package sample

import (
	"fmt"
	"net/http"
)

var version = "v1"

type Sample struct{ Name string }

var samples = []Sample{}

func NewSample(name string) int {
	fmt.Println("in NewSample", version)
	samples = append(samples, Sample{Name: name})
	i := len(samples)
	return i
}

func (s *Sample) Handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Welcome to my website", s.Name)
}

func WrapHandler(i int, w http.ResponseWriter, r *http.Request) {
	samples[i].Handler(w, r)
}

//func main() {
//	m := &Middleware{"Test"}
//	http.HandleFunc("/", Handler)
//	http.ListenAndServe(":8080", nil)
//}
