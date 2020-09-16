package main

import "github.com/traefik/yaegi/_test/c1"

func main() {
	println(c1.C1)
}

// Error:
// import cycle not allowed
//	imports github.com/traefik/yaegi/_test/c1
