package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
)

func main() {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	pub := key.Public().(*ecdsa.PublicKey)
	println(pub.Params().Name)
}

// Output:
// P-256
