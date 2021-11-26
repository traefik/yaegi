package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
)

type mw1 struct {
	next http.Handler
}

func (m *mw1) ServeHTTP(rw http.ResponseWriter, rq *http.Request) {
	m.next.ServeHTTP(rw, rq)
}

type mw0 struct{}

func (m *mw0) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Welcome to my website!")
}

func main() {
	m0 := &mw0{}
	m1 := &mw1{m0}

	mux := http.NewServeMux()
	mux.HandleFunc("/", m1.ServeHTTP)

	server := httptest.NewServer(mux)
	defer server.Close()

	client(server.URL)
}

func client(uri string) {
	resp, err := http.Get(uri)
	if err != nil {
		log.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(body))
}

// Output:
// Welcome to my website!
