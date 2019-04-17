package main

import (
	"fmt"
	"net/http"
)

type Fromage struct {
	http.Server
}

func main() {
	a := Fromage{}
	fmt.Println(a.Server)
}

// Output:
// { <nil> <nil> 0s 0s 0s 0s 0 map[] <nil> <nil> 0 0 {{0 0} 0} <nil> {0 0} map[] map[] <nil> []}
