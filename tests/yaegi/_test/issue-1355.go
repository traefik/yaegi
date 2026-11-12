package main

import "github.com/traefik/yaegi/_test/p2"

func f(i interface{}) {
	_, ok := i.(p2.I)
	println("ok:", ok)
}

func main() {
	var v *p2.T
	var i interface{}

	i = v
	_, ok := i.(p2.I)
	println("ok:", ok)
	f(v)
}

// Output:
// ok: true
// ok: true
