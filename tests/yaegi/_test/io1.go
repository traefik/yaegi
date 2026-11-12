package main

import (
	"encoding/base64"
	"fmt"
)

func main() {
	var buf [4]byte
	s := base64.RawStdEncoding.EncodeToString(buf[:])
	fmt.Println(s)
}

// Output:
// AAAAAA
