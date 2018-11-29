package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

func client() {
	resp, err := http.Get("http://localhost:8080/")
	if err != nil {
		log.Fatal(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(body))
}

func server(ready chan bool) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var r1 *http.Request = r
		fmt.Fprintln(w, "Welcome to my website!", r1)
	})

	go http.ListenAndServe(":8080", nil)
	ready <- true
}

func main() {
	ready := make(chan bool)
	go server(ready)
	<-ready
	client()
}
