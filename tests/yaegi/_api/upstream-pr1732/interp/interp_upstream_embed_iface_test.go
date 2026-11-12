package interp_test

import (
	"bytes"
	"testing"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// TestUpstream1480_1502 checks that an interpreted struct embedding a binary
// (stdlib) struct satisfies a binary interface through the promoted methods of
// its embedded field, and that calls dispatched through the interface reach
// the embedded field methods. See upstream issues #1480 (golang/glog) and
// #1502 (go-jose): "cannot use type *glog.buffer as type io.Writer".
func TestUpstream1480_1502(t *testing.T) {
	var out bytes.Buffer
	i := interp.New(interp.Options{Stdout: &out, Stderr: &out})
	if err := i.Use(stdlib.Symbols); err != nil {
		t.Fatal(err)
	}

	_, err := i.Eval(`
package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
)

type buffer struct {
	bytes.Buffer
	tmp [64]byte
}

type pbuffer struct {
	*bytes.Buffer
	n int
}

func main() {
	var buf buffer

	// Pointer form: promoted methods of the embedded binary field.
	var w io.Writer = &buf
	if _, err := w.Write([]byte("hello\n")); err != nil {
		panic(err)
	}

	// Call of a promoted method with a binary interface argument
	// (fmt.Fprintln takes an io.Writer, as glog does).
	fmt.Fprintln(&buf, "world")

	// Value form: promoted method of an embedded pointer binary field.
	var p pbuffer
	p.Buffer = &bytes.Buffer{}
	var w2 io.Writer = p
	fmt.Fprint(w2, "value form")

	// base64.NewEncoder(io.Writer), the go-jose json/encode.go path.
	e := &buffer{}
	enc := base64.NewEncoder(base64.StdEncoding, e)
	enc.Write([]byte("go-jose"))
	enc.Close()

	fmt.Printf("buf: %q b64: %s\n", buf.String(), e.String())
	fmt.Println("pbuf:", p.String())
}
`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "buf: \"hello\\nworld\\n\" b64: Z28tam9zZQ==\npbuf: value form\n"
	if res := out.String(); res != expected {
		t.Errorf("\ngot:  %q,\nwant: %q", res, expected)
	}
}
