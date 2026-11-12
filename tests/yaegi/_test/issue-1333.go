package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
)

func mock(name string) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		fmt.Fprint(rw, "Hello ", name)
	}
}

func client(uri string) {
	resp, err := http.Get(uri)
	if err != nil {
		panic(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(body))
}

func main() {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.Handle("/", mock("foo"))
	client(server.URL)
}

// Output:
// Hello foo
