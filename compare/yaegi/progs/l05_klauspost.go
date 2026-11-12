package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
)

func main() {
	in := []byte(strings.Repeat("traefik plugin ", 200))
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(in)
	w.Close()
	r, err := gzip.NewReader(&buf)
	fmt.Println(err)
	out, _ := io.ReadAll(r)
	fmt.Println(bytes.Equal(in, out), len(out))
	enc, _ := zstd.NewWriter(nil)
	c := enc.EncodeAll(in, nil)
	dec, _ := zstd.NewReader(nil)
	d, err := dec.DecodeAll(c, nil)
	fmt.Println(bytes.Equal(in, d), err)
}
