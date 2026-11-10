package main

import "crypto/rsa"

func main() {
	var pKey any = &rsa.PublicKey{}

	if _, ok := pKey.(*rsa.PublicKey); ok {
		println("ok")
	} else {
		println("nok")
	}
}

// Output:
// ok
