package main

import (
	"bufio"
	"bytes"
)

func main() {
	var buf1 = make([]byte, 1024)
	var buf2 []byte
	buf1 = []byte("Hallo\nTest\nLine3")

	s := bufio.NewScanner(bytes.NewReader(buf1))
	for s.Scan() {
		buf2 = append(buf2, append(s.Bytes(), []byte("\n")...)...)
	}
	print(string(buf2))
}

// Output:
// Hallo
// Test
// Line3
