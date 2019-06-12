package main

import (
	"crypto/sha1"
	"fmt"
)

func main() {
	d := sha1.New()
	d.Write([]byte("password"))
	a := d.Sum(nil)
	fmt.Println(a)
}

// Output:
// [91 170 97 228 201 185 63 63 6 130 37 11 108 248 51 27 126 230 143 216]
