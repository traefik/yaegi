package main

import (
	"encoding/binary"
	"fmt"
)

func main() {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(1))

	fmt.Println(b)
}

// Output:
// [1 0 0 0 0 0 0 0]
