package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
)

type T struct {
	ln net.Listener
}

func (t *T) Close() {
	t.ln.Close()
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

func server(ln net.Listener, ready chan bool) {
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		var r1 *http.Request = r
		fmt.Fprintln(w, "Welcome to my website!", r1.RequestURI)
	})

	go http.Serve(ln, nil)
	ready <- true
}

func main() {
	ln, err := net.Listen("tcp", "localhost:0")
	t := &T{ln}
	if err != nil {
		log.Fatal(err)
	}
	defer t.Close()
	//	defer ln.Close()

	ready := make(chan bool)
	go server(ln, ready)
	<-ready

	client(fmt.Sprintf("http://%s/hello", ln.Addr().String()))
	http.DefaultServeMux = &http.ServeMux{}
}

// Output:
// Welcome to my website! /hello
