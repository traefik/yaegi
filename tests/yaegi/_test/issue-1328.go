package main

import (
	"crypto/sha1"
	"encoding/hex"
)

func main() {
	script := "hello"
	sumRaw := sha1.Sum([]byte(script))
	sum := hex.EncodeToString(sumRaw[:])
	println(sum)
}

// Output:
// aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d
