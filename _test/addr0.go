package main

import (
	"fmt"
	"net/http"
)

type extendedRequest struct {
	http.Request

	Data string
}

func main() {
	r := extendedRequest{}
	req := &r.Request


	fmt.Println(r)
	fmt.Println(req)
}

// Output:
// {{ <nil>  0 0 map[] <nil> <nil> 0 [] false  map[] map[] <nil> map[]   <nil> <nil> <nil> <nil>} }
// &{ <nil>  0 0 map[] <nil> <nil> 0 [] false  map[] map[] <nil> map[]   <nil> <nil> <nil> <nil>}
