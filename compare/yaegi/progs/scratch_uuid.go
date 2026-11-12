package main

import (
	"crypto/sha1"
	"fmt"
)

type UUID [16]byte

type Version byte

func (v Version) String() string { return fmt.Sprintf("VERSION_%d", int(v)) }

func main() {
	space := UUID{1, 2, 3}
	h := sha1.New()
	h.Write(space[:])
	h.Write([]byte("x"))
	s := h.Sum(nil)
	var u UUID
	copy(u[:], s)
	fmt.Printf("%x\n", u[:4])
	fmt.Println(Version(5))
	fmt.Printf("%v %s\n", Version(5), Version(5))
}
