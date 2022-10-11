package main

import (
	"net/http"
	"net/http/httptest"
)

type T struct {
	name string
	next http.Handler
}

func (t *T) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	println("in T.ServeHTTP")
	if t.next != nil {
		t.next.ServeHTTP(rw, req)
	}
}

func New(name string, next http.Handler) (http.Handler, error) { return &T{name, next}, nil }

func main() {
	next := func(rw http.ResponseWriter, req *http.Request) {
		println("in next")
	}

	t, err := New("test", http.HandlerFunc(next))
	if err != nil {
		panic(err)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	t.ServeHTTP(recorder, req)
	println(recorder.Result().Status)
}

// Output:
// in T.ServeHTTP
// in next
// 200 OK
