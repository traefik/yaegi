package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
)

// encodeState mimics the encodeState type of go-jose json encoding: a struct
// embedding a binary (stdlib) bytes.Buffer, passed to base64.NewEncoder as an
// io.Writer. See go-jose json/encode.go and yaegi issue #1502.
type encodeState struct {
	bytes.Buffer
	scratch [64]byte
}

func main() {
	e := &encodeState{}
	enc := base64.NewEncoder(base64.StdEncoding, e)
	if _, err := enc.Write([]byte("go-jose")); err != nil {
		panic(err)
	}
	if err := enc.Close(); err != nil {
		panic(err)
	}
	fmt.Println("b64:", e.String())
}

// Output:
// b64: Z28tam9zZQ==
