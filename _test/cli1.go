package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
)

func client(uri string) {
	resp, err := http.Get(uri)
	if err != nil {
		log.Fatal(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(body))
}

func server(ln net.Listener, ready chan bool) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Welcome to my website!")
	})

	go http.Serve(ln, nil)
	ready <- true
}

func main() {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal(err)
	}

	ready := make(chan bool)
	go server(ln, ready)
	<-ready

	client(fmt.Sprintf("http://%s", ln.Addr().String()))
}

// Output:
// Welcome to my website!
