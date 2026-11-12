package main

import (
	"bufio"
	"bytes"
)

func main() {
	s := bufio.NewScanner(bytes.NewReader([]byte("Hello\nTest\nLine3")))
	s.Scan()
	println(string(append(s.Bytes(), " World"...)))
}

// Output:
// Hello World
