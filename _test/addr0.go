package main

import "net/http"

type extendedRequest struct {
	http.Request

	Data string
}

func main() {
	r := extendedRequest{}
	req := &r.Request

	println(r)
}

// Output:
// {{ <nil>  0 0 map[] <nil> <nil> 0 [] false  map[] map[] <nil> map[]   <nil> <nil> <nil> <nil>} }
