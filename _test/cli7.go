package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
)

type T struct {
	http.ResponseWriter
}

type mw1 struct {
}

var obj = map[string]interface{}{}

func (m *mw1) ServeHTTP(rw http.ResponseWriter, rq *http.Request) {
	t := &T{
		ResponseWriter: rw,
	}
	x := t.Header()
	i := obj["m1"].(*mw1)
	fmt.Fprint(rw, "Welcome to my website!", x, i)
}

func main() {
	m1 := &mw1{}

	obj["m1"] = m1

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
// Welcome to my website!map[] &{}
