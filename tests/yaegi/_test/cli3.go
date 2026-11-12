package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
)

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

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Welcome to my website!")
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client(server.URL)
}

// Output:
// Welcome to my website!
