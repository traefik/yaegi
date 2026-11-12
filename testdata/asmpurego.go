// A program run from source whose dependencies have a version in assembly
// takes their pure Go version (the purego and noasm build tags), as a plugin
// does: the VM cannot run assembly. Before, the go list loader of yaegi v2 kept
// the files of go run and the program stopped on "native func ... not in
// registry: the function has no Go body".
package main

import (
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/blake2b"
)

func main() {
	sum := blake2b.Sum256([]byte("hello"))
	fmt.Printf("blake2b %x\n", sum)

	h, _ := blake2b.New512([]byte("key"))
	h.Write([]byte("a longer message, more than one block of 128 bytes: "))
	h.Write(make([]byte, 200))
	fmt.Printf("blake2b-512 %x\n", h.Sum(nil))

	fmt.Printf("argon2 %x\n", argon2.IDKey([]byte("pw"), []byte("somesalt"), 1, 64, 1, 16))
}
